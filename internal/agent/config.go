package agent

import "strings"

// Config holds agent runtime settings. The agent is disabled when APIKey is empty.
type Config struct {
	APIKey           string
	Model            string
	Effort           string // reasoning effort: low|medium|high|xhigh|max
	MaxIterations    int    // bound on execute-loop model turns per message (rule 2)
	DailyTokenBudget int64  // per-tenant hard ceiling
	RatePerMinute    int    // per-user message rate limit
	AwaitTTLMinutes  int    // how long an awaiting risky step stays valid
}

// validEfforts is the set the Anthropic API accepts for output_config.effort.
var validEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

// ValidEffort reports whether e is an accepted effort level.
func ValidEffort(e string) bool { return validEfforts[e] }

// SupportsEffort reports whether model accepts output_config.effort. Haiku-tier
// models reject it (HTTP 400), so effort must be omitted for them.
func SupportsEffort(model string) bool {
	return !strings.Contains(model, "haiku")
}

// EffortFor returns the effort to send for model: the configured value when the
// model supports effort, or "" (omit) when it does not.
func (c Config) EffortFor() string {
	if !SupportsEffort(c.Model) {
		return ""
	}
	return c.Effort
}

func (c Config) Enabled() bool { return c.APIKey != "" }

// WithDefaults fills unset fields with sensible defaults. An unset or invalid
// Effort falls back to "high" so a bad env value can never reach the API.
func (c Config) WithDefaults() Config {
	if c.Model == "" {
		c.Model = "claude-opus-4-8"
	}
	if !ValidEffort(c.Effort) {
		c.Effort = "high"
	}
	if c.MaxIterations == 0 {
		c.MaxIterations = 25
	}
	if c.DailyTokenBudget == 0 {
		c.DailyTokenBudget = 2_000_000
	}
	if c.RatePerMinute == 0 {
		c.RatePerMinute = 20
	}
	if c.AwaitTTLMinutes == 0 {
		c.AwaitTTLMinutes = 30
	}
	return c
}
