// Package mcp is a minimal Model Context Protocol server over HTTP
// (JSON-RPC, single response per request). It exposes Vigia's capabilities
// as tools to an exploring agent.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
)

type Tool struct {
	Name        string                                                       `json:"name"`
	Description string                                                       `json:"description"`
	InputSchema json.RawMessage                                              `json:"inputSchema"`
	Handle      func(ctx context.Context, args json.RawMessage) (any, error) `json:"-"`
}

// Server serves one set of tools. Build one per exploration.
type Server struct {
	Name  string
	Tools []Tool
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		reply(w, nil, nil, &rpcError{-32700, "parse error"})
		return
	}
	if len(req.ID) == 0 {
		// Notifications (e.g. notifications/initialized) get no body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch req.Method {
	case "initialize":
		reply(w, req.ID, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": s.Name, "version": "1"},
		}, nil)
	case "ping":
		reply(w, req.ID, map[string]any{}, nil)
	case "tools/list":
		reply(w, req.ID, map[string]any{"tools": s.Tools}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(w, req.ID, nil, &rpcError{-32602, "invalid params"})
			return
		}
		for _, t := range s.Tools {
			if t.Name != p.Name {
				continue
			}
			out, err := t.Handle(r.Context(), p.Arguments)
			if err != nil {
				reply(w, req.ID, map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}, nil)
				return
			}
			b, _ := json.Marshal(out)
			reply(w, req.ID, map[string]any{"content": []map[string]string{{"type": "text", "text": string(b)}}}, nil)
			return
		}
		reply(w, req.ID, nil, &rpcError{-32602, "unknown tool " + p.Name})
	default:
		reply(w, req.ID, nil, &rpcError{-32601, "method not found: " + req.Method})
	}
}

func reply(w http.ResponseWriter, id json.RawMessage, result any, e *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	msg := map[string]any{"jsonrpc": "2.0", "id": id}
	if e != nil {
		msg["error"] = e
	} else {
		msg["result"] = result
	}
	json.NewEncoder(w).Encode(msg)
}
