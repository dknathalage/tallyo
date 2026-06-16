package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic implements Client.
var _ Client = (*Anthropic)(nil)

// Anthropic adapts the anthropic-sdk-go Messages API to the provider-agnostic
// Client interface. All anthropic-sdk-go symbols are confined to this file.
type Anthropic struct {
	c      anthropic.Client
	model  string
	effort string
}

// NewAnthropic builds an Anthropic client. model defaults the request model when
// Request.Model is empty; effort (one of low/medium/high/xhigh/max) sets
// output_config.effort when non-empty.
func NewAnthropic(apiKey, model, effort string) *Anthropic {
	return &Anthropic{
		c:      anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
		effort: effort,
	}
}

// CreateMessage sends one request to the Messages API and maps the result back
// into the provider-agnostic Response shape.
func (a *Anthropic) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	if req.MaxTokens <= 0 {
		return nil, fmt.Errorf("llm: MaxTokens must be positive, got %d", req.MaxTokens)
	}

	model := a.model
	if req.Model != "" {
		model = req.Model
	}
	if model == "" {
		return nil, fmt.Errorf("llm: model not configured")
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  toSDKMessages(req.Messages),
		// Adaptive thinking is the only supported on-mode for current Opus/Fable models.
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}

	effort := a.effort
	if req.Effort != "" {
		effort = req.Effort
	}
	if effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(effort),
		}
	}

	tools, err := toSDKTools(req.Tools)
	if err != nil {
		return nil, err
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.ToolChoice.ForceTool != "" {
		params.ToolChoice = anthropic.ToolChoiceParamOfTool(req.ToolChoice.ForceTool)
	}

	resp, err := a.c.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm: anthropic create message: %w", err)
	}
	return fromSDK(resp), nil
}

// toSDKTools converts our ToolDef list into SDK tool unions, decoding the JSON
// Schema input into Properties/Required.
func toSDKTools(defs []ToolDef) ([]anthropic.ToolUnionParam, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		if d.Name == "" {
			return nil, fmt.Errorf("llm: tool with empty name")
		}
		schema := anthropic.ToolInputSchemaParam{}
		if len(d.InputSchema) > 0 {
			var parsed struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			if err := json.Unmarshal(d.InputSchema, &parsed); err != nil {
				return nil, fmt.Errorf("llm: tool %q input schema: %w", d.Name, err)
			}
			schema.Properties = parsed.Properties
			schema.Required = parsed.Required
		}
		tool := anthropic.ToolParam{Name: d.Name, InputSchema: schema}
		if d.Description != "" {
			tool.Description = anthropic.String(d.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out, nil
}

// toSDKMessages converts our messages (request direction) into SDK MessageParams.
func toSDKMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.Content)+len(m.ToolResults))
		for _, b := range m.Content {
			switch b.Type {
			case BlockText, BlockThinking:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case BlockToolUse:
				var input any
				if len(b.Input) > 0 {
					input = json.RawMessage(b.Input)
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(b.ToolUseID, input, b.ToolName))
			}
		}
		for _, tr := range m.ToolResults {
			blocks = append(blocks, anthropic.NewToolResultBlock(tr.ToolUseID, tr.Content, tr.IsError))
		}

		if m.Role == RoleAssistant {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		} else {
			out = append(out, anthropic.NewUserMessage(blocks...))
		}
	}
	return out
}

// fromSDK maps an SDK response Message back into our Response.
func fromSDK(m *anthropic.Message) *Response {
	resp := &Response{
		StopReason: mapStopReason(m.StopReason),
		Content:    make([]Block, 0, len(m.Content)),
		Usage: Usage{
			InputTokens:      m.Usage.InputTokens,
			OutputTokens:     m.Usage.OutputTokens,
			CacheReadTokens:  m.Usage.CacheReadInputTokens,
			CacheWriteTokens: m.Usage.CacheCreationInputTokens,
		},
	}
	for _, b := range m.Content {
		switch v := b.AsAny().(type) {
		case anthropic.TextBlock:
			resp.Content = append(resp.Content, Block{Type: BlockText, Text: v.Text})
		case anthropic.ThinkingBlock:
			resp.Content = append(resp.Content, Block{Type: BlockThinking, Text: v.Thinking})
		case anthropic.ToolUseBlock:
			resp.Content = append(resp.Content, Block{
				Type:      BlockToolUse,
				ToolUseID: v.ID,
				ToolName:  v.Name,
				Input:     json.RawMessage(v.Input),
			})
		}
	}
	return resp
}

// mapStopReason translates SDK stop reasons to our stop constants, passing
// unknown values through as their raw string.
func mapStopReason(r anthropic.StopReason) string {
	switch r {
	case anthropic.StopReasonRefusal:
		return StopRefusal
	case anthropic.StopReasonToolUse:
		return StopToolUse
	case anthropic.StopReasonEndTurn:
		return StopEndTurn
	case anthropic.StopReasonMaxTokens:
		return StopMaxTok
	default:
		return string(r)
	}
}
