package agent

import "github.com/dknathalage/tallyo/internal/agent/llm"

// maxHistoryMessages caps how many trailing conversation messages are replayed
// into a request. Compaction/summarization is a later phase; for Phase 1 we
// simply window the most recent messages (rule 2: bounded).
const maxHistoryMessages = 40

// buildRequest assembles an llm.Request for a turn. It windows history to the
// last maxHistoryMessages messages, exposes every tool (meta included so the
// model can see propose_plan), and forces the named tool when force != "".
func buildRequest(cfg Config, reg *Registry, system string, history []llm.Message, force string) llm.Request {
	if reg == nil {
		panic("agent: buildRequest requires a non-nil registry")
	}
	if system == "" {
		system = SystemPrompt()
	}
	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	return llm.Request{
		System:     system,
		Tools:      reg.Defs(true),
		ToolChoice: llm.ToolChoice{ForceTool: force},
		Messages:   history,
		MaxTokens:  requestMaxTokens,
		Model:      cfg.Model,
		Effort:     cfg.EffortFor(), // omitted for Haiku-tier models
	}
}
