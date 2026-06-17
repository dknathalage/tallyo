package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// hasEphemeralCacheControl reports whether marshaling v produces a
// `"cache_control":{"type":"ephemeral"}` breakpoint. The SDK marshals the
// CacheControl field with `omitzero`, so an unset breakpoint produces no
// cache_control key at all.
func hasEphemeralCacheControl(t *testing.T, v any) bool {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return strings.Contains(string(b), `"cache_control":{"type":"ephemeral"}`)
}

// buildSystemBlocks puts a cache breakpoint on the (single) system block.
func TestBuildSystemBlocksAddsCacheBreakpoint(t *testing.T) {
	blocks := buildSystemBlocks("you are a helpful assistant")
	if len(blocks) != 1 {
		t.Fatalf("want 1 system block, got %d", len(blocks))
	}
	if !hasEphemeralCacheControl(t, blocks[len(blocks)-1]) {
		t.Fatalf("last system block missing ephemeral cache_control")
	}
}

// buildSystemBlocks returns nil for an empty prompt (no breakpoint).
func TestBuildSystemBlocksEmpty(t *testing.T) {
	if blocks := buildSystemBlocks(""); blocks != nil {
		t.Fatalf("want nil for empty system, got %v", blocks)
	}
}

// toSDKTools puts a cache breakpoint on the LAST tool only.
func TestToSDKToolsCacheBreakpointOnLastTool(t *testing.T) {
	defs := []ToolDef{
		{Name: "first", InputSchema: json.RawMessage(`{"properties":{},"required":[]}`)},
		{Name: "middle", InputSchema: json.RawMessage(`{"properties":{},"required":[]}`)},
		{Name: "last", InputSchema: json.RawMessage(`{"properties":{},"required":[]}`)},
	}
	tools, err := toSDKTools(defs)
	if err != nil {
		t.Fatalf("toSDKTools: %v", err)
	}
	if len(tools) != len(defs) {
		t.Fatalf("want %d tools, got %d", len(defs), len(tools))
	}
	for i, tu := range tools {
		got := hasEphemeralCacheControl(t, tu.OfTool)
		wantLast := i == len(tools)-1
		if got != wantLast {
			t.Fatalf("tool[%d] (%q) cache_control=%v, want %v", i, defs[i].Name, got, wantLast)
		}
	}
}

// toSDKTools returns nil (no breakpoint) for an empty tool list.
func TestToSDKToolsEmpty(t *testing.T) {
	tools, err := toSDKTools(nil)
	if err != nil {
		t.Fatalf("toSDKTools: %v", err)
	}
	if tools != nil {
		t.Fatalf("want nil for empty tools, got %v", tools)
	}
}
