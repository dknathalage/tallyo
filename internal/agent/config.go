package agent

// Config holds agent runtime settings. The agent is disabled when APIKey is empty.
type Config struct {
	APIKey           string
	Model            string
	MaxIterations    int   // bound on execute-loop model turns per message (rule 2)
	DailyTokenBudget int64 // per-tenant hard ceiling
	RatePerMinute    int   // per-user message rate limit
	AwaitTTLMinutes  int   // how long an awaiting risky step stays valid
}

func (c Config) Enabled() bool { return c.APIKey != "" }

// WithDefaults fills unset fields with sensible defaults.
func (c Config) WithDefaults() Config {
	if c.Model == "" {
		c.Model = "claude-opus-4-8"
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
