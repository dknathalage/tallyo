package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// Risk classifies a tool's mutation profile so the permission gate can decide
// whether to ask the user before running it.
type Risk string

const (
	RiskRead  Risk = "read"
	RiskRisky Risk = "risky"
	RiskMeta  Risk = "meta"
)

// Result is a tool's structured output plus a UI render hint.
type Result struct {
	JSON   any
	Render string // "table" | "card" | "summary"
}

// Tool is one capability the agent may call. Handlers must call services only;
// never reach directly into repositories or the DB.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Risk        Risk
	Render      string
	Handler     func(ctx context.Context, input json.RawMessage) (Result, error)
}

// Registry holds all registered tools, keyed by name.
type Registry struct{ tools map[string]Tool }

// NewRegistry returns an empty, ready-to-use tool registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds a tool, panicking on invalid or duplicate registration. Use
// this at startup where a configuration error must be fatal.
func (r *Registry) Register(t Tool) {
	if err := r.register(t); err != nil {
		panic(err)
	}
}

// register validates and inserts a tool, returning an error on invalid or
// duplicate input. Called by Register (panics) and the duplicate test.
func (r *Registry) register(t Tool) error {
	if t.Name == "" || t.Handler == nil {
		return fmt.Errorf("registry: tool needs name and handler")
	}
	if t.Risk != RiskRead && t.Risk != RiskRisky && t.Risk != RiskMeta {
		return fmt.Errorf("registry: tool %q invalid risk %q", t.Name, t.Risk)
	}
	if _, dup := r.tools[t.Name]; dup {
		return fmt.Errorf("registry: duplicate tool %q", t.Name)
	}
	r.tools[t.Name] = t
	return nil
}

// Get returns the named tool and whether it was found.
func (r *Registry) Get(name string) (Tool, bool) { t, ok := r.tools[name]; return t, ok }

// Defs returns tool definitions for the model. Meta tools (e.g. propose_plan)
// are excluded unless includeMeta is true.
func (r *Registry) Defs(includeMeta bool) []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		if t.Risk == RiskMeta && !includeMeta {
			continue
		}
		defs = append(defs, llm.ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}
	return defs
}
