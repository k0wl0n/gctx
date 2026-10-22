package provider

import (
	"fmt"
	"sort"
)

// canonicalOrder ranks the built-in providers; others follow sorted by ID.
var canonicalOrder = []ID{GCP, AWS, Azure, Tencent}

// Registry holds providers keyed by ID.
type Registry struct {
	byID    map[ID]Provider
	ordered []Provider
}

// NewRegistry panics on a duplicate ID, which is a programming error.
func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{byID: make(map[ID]Provider, len(providers))}
	for _, p := range providers {
		if _, exists := registry.byID[p.ID()]; exists {
			panic(fmt.Sprintf("provider: duplicate id %q", p.ID()))
		}
		registry.byID[p.ID()] = p
		registry.ordered = append(registry.ordered, p)
	}
	sort.SliceStable(registry.ordered, func(i, j int) bool {
		return orderKey(registry.ordered[i].ID()) < orderKey(registry.ordered[j].ID())
	})
	return registry
}

func orderKey(id ID) string {
	for rank, known := range canonicalOrder {
		if id == known {
			return fmt.Sprintf("0%d", rank)
		}
	}
	return "1" + string(id)
}

func (r *Registry) Get(id ID) (Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

// All returns providers in stable order: gcp, aws, azure, tencent, then others by ID.
func (r *Registry) All() []Provider {
	return append([]Provider(nil), r.ordered...)
}
