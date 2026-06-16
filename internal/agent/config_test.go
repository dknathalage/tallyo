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
}
