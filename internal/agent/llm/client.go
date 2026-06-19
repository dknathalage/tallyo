// Package llm is a provider-agnostic wrapper over the chat-completions API the
// agent loop drives. Keeping it behind an interface makes the loop testable with
// a scripted fake and isolates the anthropic-sdk-go surface to one file.
package llm

import (
	"context"
	"encoding/json"
)

type BlockType string

const (
	BlockText     BlockType = "text"
	BlockToolUse  BlockType = "tool_use"
	BlockThinking BlockType = "thinking"
)

const (
	StopEndTurn = "end_turn"
	StopToolUse = "tool_use"
	StopMaxTok  = "max_tokens"
	StopRefusal = "refusal"
)

// Block is one content block in a request or response message.
type Block struct {
	Type      BlockType
	Text      string          // BlockText / BlockThinking
	ToolUseID string          // BlockToolUse
	ToolName  string          // BlockToolUse
	Input     json.RawMessage // BlockToolUse input
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role
	Content []Block
	// ToolResults carries tool_result blocks for a user turn answering tool_use.
	ToolResults []ToolResult
}

type ToolResult struct {
	ToolUseID string
	Content   string // JSON-encoded tool output
	IsError   bool
}

type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema
}

// ToolChoice forces a specific tool (plan phase) or leaves it auto.
type ToolChoice struct {
	ForceTool string // "" = auto
}

type Request struct {
	System     string
	Tools      []ToolDef
	ToolChoice ToolChoice
	Messages   []Message
	MaxTokens  int
	Model      string
	Effort     string // "high"
}

type Response struct {
	StopReason string
	Content    []Block
}

// Client is the single dependency the agent loop has on the model provider.
type Client interface {
	CreateMessage(ctx context.Context, req Request) (*Response, error)
}
