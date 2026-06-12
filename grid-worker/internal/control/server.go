package control

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/rs/zerolog"
)

// CommandHandler is a function that processes a command payload and returns a response data value.
type CommandHandler func(payload json.RawMessage) (any, error)

// Command is an inbound IPC message.
type Command struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Response is an outbound IPC message.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Server is a local IPC server using a platform-appropriate socket.
// Protocol: 4-byte big-endian length prefix + JSON body.
type Server struct {
	socketPath string
	handlers   map[string]CommandHandler
	mu         sync.RWMutex
	listener   net.Listener
	log        zerolog.Logger
}

// New creates a new Server.
func New(socketPath string, log zerolog.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		handlers:   make(map[string]CommandHandler),
		log:        log.With().Str("component", "control-server").Logger(),
	}
}

// Register adds a command handler for the given command type.
func (s *Server) Register(cmdType string, handler CommandHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[cmdType] = handler
}

// Start begins accepting connections on the socket. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	ln, err := Listen(s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", s.socketPath, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.log.Info().Str("socket", s.socketPath).Msg("control server listening")

	// Accept connections until context is cancelled
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil // clean shutdown
			default:
				s.log.Error().Err(err).Msg("accept error")
				return err
			}
		}
		go s.handleConn(conn)
	}
}

// Stop closes the listener.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		s.listener.Close()
	}
}

// handleConn processes a single client connection.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		// Read length-prefixed message
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			if err != io.EOF {
				s.log.Debug().Err(err).Msg("read length prefix")
			}
			return
		}

		if length == 0 || length > 1<<20 {
			s.log.Warn().Uint32("length", length).Msg("invalid message length")
			return
		}

		buf := make([]byte, length)
		if _, err := io.ReadFull(conn, buf); err != nil {
			s.log.Debug().Err(err).Msg("read message body")
			return
		}

		var cmd Command
		if err := json.Unmarshal(buf, &cmd); err != nil {
			s.writeError(conn, "invalid JSON")
			return
		}

		resp := s.dispatch(cmd)
		if err := s.writeResponse(conn, resp); err != nil {
			s.log.Debug().Err(err).Msg("write response")
			return
		}
	}
}

// dispatch routes a command to its registered handler.
func (s *Server) dispatch(cmd Command) Response {
	s.mu.RLock()
	handler, ok := s.handlers[cmd.Type]
	s.mu.RUnlock()

	if !ok {
		return Response{
			OK:    false,
			Error: fmt.Sprintf("unknown command type %q", cmd.Type),
		}
	}

	result, err := handler(cmd.Payload)
	if err != nil {
		return Response{OK: false, Error: err.Error()}
	}

	if result == nil {
		return Response{OK: true}
	}

	data, jsonErr := json.Marshal(result)
	if jsonErr != nil {
		return Response{OK: false, Error: fmt.Sprintf("marshal response data: %v", jsonErr)}
	}

	return Response{OK: true, Data: data}
}

// writeResponse sends a length-prefixed JSON response.
func (s *Server) writeResponse(w io.Writer, resp Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write body: %w", err)
	}

	return nil
}

// writeError sends an error response.
func (s *Server) writeError(conn net.Conn, msg string) {
	resp := Response{OK: false, Error: msg}
	_ = s.writeResponse(conn, resp)
}

// SendCommand connects to a control server and sends a single command, returning the response.
// This is used by CLI commands to communicate with the running daemon.
func SendCommand(socketPath string, cmd Command) (*Response, error) {
	conn, err := Connect(socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to control socket %q: %w", socketPath, err)
	}
	defer conn.Close()

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}

	if err := binary.Write(conn, binary.BigEndian, uint32(len(data))); err != nil {
		return nil, fmt.Errorf("write length: %w", err)
	}

	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("write command: %w", err)
	}

	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("read response length: %w", err)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(buf, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp, nil
}
