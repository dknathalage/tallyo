package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// propose forces the model to emit exactly one tool_use whose input matches the
// schema, and decodes it into T. One call — no conversation, no history. The
// model's only job is to fill the schema; deterministic Go owns everything after.
func propose[T any](ctx context.Context, c llm.Client, cfg Config,
	system, userContent, toolName string, schema json.RawMessage) (T, error) {

	var zero T
	req := llm.Request{
		System:     system,
		Tools:      []llm.ToolDef{{Name: toolName, InputSchema: schema}},
		ToolChoice: llm.ToolChoice{ForceTool: toolName},
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.Block{{Type: llm.BlockText, Text: userContent}},
		}},
		MaxTokens: requestMaxTokens,
		Model:     cfg.Model,
		Effort:    cfg.EffortFor(),
	}
	resp, err := c.CreateMessage(ctx, req)
	if err != nil {
		return zero, fmt.Errorf("propose %s: %w", toolName, err)
	}
	if resp == nil {
		return zero, fmt.Errorf("propose %s: nil response", toolName)
	}
	for i := range resp.Content { // bounded by len(Content)
		b := resp.Content[i]
		if b.Type == llm.BlockToolUse && b.ToolName == toolName {
			var out T
			if e := json.Unmarshal(b.Input, &out); e != nil {
				return zero, fmt.Errorf("propose %s: decode input: %w", toolName, e)
			}
			return out, nil
		}
	}
	return zero, fmt.Errorf("propose %s: model emitted no %s call", toolName, toolName)
}
