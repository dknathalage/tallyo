package subscription

import "testing"

func TestLoadConfigDefaults(t *testing.T) {
	// t.Setenv ensures these are unset for the duration of the test.
	t.Setenv("BILLING_ENABLED", "")
	t.Setenv("TRIAL_DAYS", "")
	c := LoadConfig()
	if c.Enabled {
		t.Error("default Enabled should be false")
	}
	if c.TrialDays != DefaultTrialDays {
		t.Errorf("default TrialDays = %d, want %d", c.TrialDays, DefaultTrialDays)
	}
}

func TestLoadConfigTrialDays(t *testing.T) {
	t.Setenv("TRIAL_DAYS", "14")
	if c := LoadConfig(); c.TrialDays != 14 {
		t.Errorf("TrialDays = %d, want 14", c.TrialDays)
	}
}

func TestLoadConfigTrialDaysInvalidFallsBack(t *testing.T) {
	t.Setenv("TRIAL_DAYS", "notanumber")
	if c := LoadConfig(); c.TrialDays != DefaultTrialDays {
		t.Errorf("TrialDays = %d, want fallback %d", c.TrialDays, DefaultTrialDays)
	}
}

func TestLoadConfigEnabled(t *testing.T) {
	t.Setenv("BILLING_ENABLED", "true")
	if c := LoadConfig(); !c.Enabled {
		t.Error("Enabled should be true when BILLING_ENABLED=true")
	}
}
