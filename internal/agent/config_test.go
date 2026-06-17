package agent

import "testing"

func TestConfigEnabled(t *testing.T) {
	if (Config{APIKey: ""}).Enabled() {
		t.Fatal("empty key must be disabled")
	}
	if !(Config{APIKey: "sk-x"}).Enabled() {
		t.Fatal("non-empty key must be enabled")
	}
}

func TestConfigDefaults(t *testing.T) {
	c := Config{APIKey: "sk-x"}.WithDefaults()
	if c.Model == "" || c.MaxIterations == 0 || c.DailyTokenBudget == 0 {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if c.Effort != "high" {
		t.Fatalf("unset effort must default to high, got %q", c.Effort)
	}
}

func TestConfigEffort(t *testing.T) {
	// Valid effort from env is preserved.
	if c := (Config{APIKey: "sk-x", Effort: "low"}).WithDefaults(); c.Effort != "low" {
		t.Fatalf("valid effort must be kept, got %q", c.Effort)
	}
	// Invalid effort falls back to the default — a bad env value never reaches the API.
	if c := (Config{APIKey: "sk-x", Effort: "bogus"}).WithDefaults(); c.Effort != "high" {
		t.Fatalf("invalid effort must fall back to high, got %q", c.Effort)
	}
	if ValidEffort("bogus") || !ValidEffort("xhigh") {
		t.Fatal("ValidEffort set is wrong")
	}
}

func TestEffortFor(t *testing.T) {
	// Opus supports effort — the configured value is sent.
	opus := Config{APIKey: "sk-x", Model: "claude-opus-4-8", Effort: "high"}.WithDefaults()
	if opus.EffortFor() != "high" {
		t.Fatalf("opus must send effort, got %q", opus.EffortFor())
	}
	// Haiku rejects effort — it must be omitted regardless of config.
	haiku := Config{APIKey: "sk-x", Model: "claude-haiku-4-5", Effort: "high"}.WithDefaults()
	if haiku.EffortFor() != "" {
		t.Fatalf("haiku must omit effort, got %q", haiku.EffortFor())
	}
	if SupportsEffort("claude-haiku-4-5") || !SupportsEffort("claude-opus-4-8") {
		t.Fatal("SupportsEffort is wrong")
	}
}
