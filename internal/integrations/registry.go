// Package integrations provides a registry scaffold for third-party
// service clients. v1 only ships Deepgram; the registry exists so new
// integrations (Linear, GitHub, etc.) can plug in without refactoring
// callers.
package integrations

import (
	"fmt"
	"sync"
)

// Integration is the minimum contract a third-party service client must satisfy.
type Integration interface {
	// ID is a short stable identifier (e.g., "deepgram", "linear").
	ID() string

	// Name is the display name (e.g., "Deepgram").
	Name() string

	// Configured reports whether the integration has the credentials it needs.
	Configured() bool

	// HealthCheck verifies the integration can talk to its remote service.
	// Called lazily; may be a no-op for integrations without a cheap ping.
	HealthCheck() error
}

// Registry holds all registered integrations by ID.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Integration
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{items: make(map[string]Integration)}
}

// Register adds an integration. Panics if an integration with the same ID
// already exists — catch this in tests, never in production.
func (r *Registry) Register(i Integration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[i.ID()]; exists {
		panic(fmt.Sprintf("integrations: duplicate registration for %q", i.ID()))
	}
	r.items[i.ID()] = i
}

// Get returns the integration with the given ID, or nil if not registered.
func (r *Registry) Get(id string) Integration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[id]
}

// All returns a snapshot of every registered integration.
func (r *Registry) All() []Integration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Integration, 0, len(r.items))
	for _, i := range r.items {
		out = append(out, i)
	}
	return out
}

// Status reports each integration's ID, name, and configured state.
type Status struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// Statuses returns a quick summary of every registered integration,
// useful for the /api/integrations endpoint.
func (r *Registry) Statuses() []Status {
	items := r.All()
	out := make([]Status, 0, len(items))
	for _, i := range items {
		out = append(out, Status{
			ID:         i.ID(),
			Name:       i.Name(),
			Configured: i.Configured(),
		})
	}
	return out
}
