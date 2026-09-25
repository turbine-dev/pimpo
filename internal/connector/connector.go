// Package connector links capabilities to real services. Each connector
// serves a few capabilities; the Router sends every call to its owner.
package connector

import (
	"context"
	"encoding/json"
	"fmt"
)

type Connector interface {
	// Capabilities lists the capability names this connector serves.
	Capabilities() []string
	Call(ctx context.Context, capability, scope string, args any) (any, error)
}

type Router struct {
	owners map[string]Connector
}

func NewRouter(cs ...Connector) *Router {
	r := &Router{owners: map[string]Connector{}}
	for _, c := range cs {
		r.Add(c)
	}
	return r
}

func (r *Router) Add(c Connector) {
	for _, name := range c.Capabilities() {
		r.owners[name] = c
	}
}

// Remove drops capabilities, for a connector that was uninstalled.
func (r *Router) Remove(names ...string) {
	for _, n := range names {
		delete(r.owners, n)
	}
}

func (r *Router) Has(capability string) bool { _, ok := r.owners[capability]; return ok }

func (r *Router) Call(ctx context.Context, capability, scope string, args any) (any, error) {
	c, ok := r.owners[capability]
	if !ok {
		return nil, fmt.Errorf("%s is not connected yet; set it up in Connections", capability)
	}
	return c.Call(ctx, capability, scope, args)
}

// Args converts loosely typed routine arguments into a struct.
func Args(in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("bad arguments: %w", err)
	}
	return nil
}
