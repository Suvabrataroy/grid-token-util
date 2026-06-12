// Package dashboard provides the Server-Sent Events hub for real-time updates.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Event is a typed message broadcast over SSE.
type Event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Client represents a single SSE connection.
type Client struct {
	ch       chan Event
	orgID    uuid.UUID
	clientID uuid.UUID
}

// Hub manages the set of active SSE clients and fans events out to them.
type Hub struct {
	mu         sync.RWMutex
	// clients is a two-level map: orgID → clientID → *Client
	clients    map[uuid.UUID]map[uuid.UUID]*Client
	maxClients int
}

// NewHub creates a Hub with the specified maximum number of SSE clients.
func NewHub(maxClients int) *Hub {
	return &Hub{
		clients:    make(map[uuid.UUID]map[uuid.UUID]*Client),
		maxClients: maxClients,
	}
}

// Register creates and registers a new SSE client for the given org unit.
// Returns nil if the max client limit has been reached.
func (h *Hub) Register(orgID uuid.UUID) *Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Count total clients across all orgs.
	total := 0
	for _, m := range h.clients {
		total += len(m)
	}
	if total >= h.maxClients {
		log.Warn().Int("limit", h.maxClients).Msg("hub: max SSE clients reached")
		return nil
	}

	c := &Client{
		ch:       make(chan Event, 32),
		orgID:    orgID,
		clientID: uuid.New(),
	}

	if h.clients[orgID] == nil {
		h.clients[orgID] = make(map[uuid.UUID]*Client)
	}
	h.clients[orgID][c.clientID] = c
	return c
}

// Unregister removes a client and closes its event channel.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if m, ok := h.clients[c.orgID]; ok {
		delete(m, c.clientID)
		if len(m) == 0 {
			delete(h.clients, c.orgID)
		}
	}
	close(c.ch)
}

// Broadcast sends an event to all connected clients for the given org unit.
func (h *Hub) Broadcast(orgID uuid.UUID, event Event) {
	h.mu.RLock()
	clients := h.clients[orgID]
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.ch <- event:
		default:
			// Client is too slow; drop the event rather than blocking.
			log.Warn().
				Str("client_id", c.clientID.String()).
				Str("event_type", event.Type).
				Msg("hub: dropped event for slow client")
		}
	}
}

// BroadcastAll sends an event to every connected client regardless of org unit.
// Use this for system-wide security alerts.
func (h *Hub) BroadcastAll(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, m := range h.clients {
		for _, c := range m {
			select {
			case c.ch <- event:
			default:
				log.Warn().
					Str("client_id", c.clientID.String()).
					Msg("hub: dropped global event for slow client")
			}
		}
	}
}

// ServeSSE handles an SSE connection for the given org unit.  It blocks until
// the client disconnects or the request context is cancelled.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request, orgID uuid.UUID) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	c := h.Register(orgID)
	if c == nil {
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	defer h.Unregister(c)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Send a connected event.
	writeSSEEvent(w, flusher, Event{
		ID:   uuid.NewString(),
		Type: "connected",
		Data: map[string]string{"client_id": c.clientID.String()},
	})

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-c.ch:
			if !open {
				return
			}
			writeSSEEvent(w, flusher, ev)
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev Event) {
	dataBytes, err := json.Marshal(ev.Data)
	if err != nil {
		log.Warn().Err(err).Msg("hub: marshal event data")
		return
	}

	if ev.ID != "" {
		fmt.Fprintf(w, "id: %s\n", ev.ID)
	}
	fmt.Fprintf(w, "event: %s\n", ev.Type)
	fmt.Fprintf(w, "data: %s\n\n", string(dataBytes))
	flusher.Flush()
}
