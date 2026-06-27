package subscription

import (
	"os"
	"strconv"
	"strings"
)

// Config is the billing configuration read from the environment. When Enabled is
// false the whole feature is dark: routes are not mounted and every tenant is
// treated as entitled (see the wiring in internal/app).
type Config struct {
	Enabled       bool
	SecretKey     string // STRIPE_SECRET_KEY
	WebhookSecret string // STRIPE_WEBHOOK_SECRET
	PriceID       string // STRIPE_PRICE_ID — the monthly recurring price the trial subscribes to
	PriceIDAnnual string // STRIPE_PRICE_ID_ANNUAL — the annual price; falls back to PriceID when unset
	TrialDays     int    // TRIAL_DAYS — passed to Stripe as trial_period_days
}

// DefaultTrialDays is used when TRIAL_DAYS is unset or unparseable.
const DefaultTrialDays = 30

// LoadConfig reads billing config from the environment.
//
// ponytail: local env reads (not app.EnvOr) — internal/app imports this package,
// so importing app back would be a cycle.
func LoadConfig() Config {
	return Config{
		Enabled:       strings.EqualFold(strings.TrimSpace(os.Getenv("BILLING_ENABLED")), "true"),
		SecretKey:     strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
		PriceID:       strings.TrimSpace(os.Getenv("STRIPE_PRICE_ID")),
		PriceIDAnnual: strings.TrimSpace(os.Getenv("STRIPE_PRICE_ID_ANNUAL")),
		TrialDays:     trialDays(os.Getenv("TRIAL_DAYS")),
	}
}

func trialDays(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return DefaultTrialDays
	}
	return n
}
