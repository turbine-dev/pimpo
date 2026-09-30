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

// MissingCredential says a connector cannot work without a password or key
// its person has not given, or that the service refused (Invalid). Only
// Pimpo's code builds one, never a model; the app turns it into a private
// request that the person answers in a form of its own.
type MissingCredential struct {
	// Connector is the catalog kind or the external connector's name.
	Connector string
	// Field is the connector's field or environment variable.
	Field   string
	Invalid bool
	// Err is what went wrong, as the connector said it.
	Err error
}

func (m *MissingCredential) Error() string {
	if m.Err != nil {
		return m.Err.Error()
	}
	return m.Connector + " needs " + m.Field
}

func (m *MissingCredential) Unwrap() error { return m.Err }
