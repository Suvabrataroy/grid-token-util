package adapters

import "sort"

// Registry holds all registered agent adapters, keyed by their ID.
type Registry struct {
	adapters map[string]AgentAdapter
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]AgentAdapter),
	}
}

// Register adds an adapter to the registry. If an adapter with the same ID
// already exists it is replaced.
func (r *Registry) Register(a AgentAdapter) {
	r.adapters[a.ID()] = a
}

// Get retrieves an adapter by its ID. Returns (adapter, true) if found,
// or (nil, false) if not.
func (r *Registry) Get(id string) (AgentAdapter, bool) {
	a, ok := r.adapters[id]
	return a, ok
}

// List returns the sorted list of all registered adapter IDs.
func (r *Registry) List() []string {
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
