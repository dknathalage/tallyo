package subscription

import (
	"context"
	"errors"
	"fmt"

	stripe "github.com/stripe/stripe-go/v82"
)

// Client wraps the Stripe SDK with the few calls this app makes: create a
// Checkout Session (start the trial), create a Customer Portal session (manage),
// and fetch a subscription (webhook self-heal). The webhook signature secret is
// kept here too so the handler has one place to read config-derived Stripe state.
type Client struct {
	api           *stripe.Client
	priceID       string // monthly
	priceIDAnnual string // annual; equals priceID when STRIPE_PRICE_ID_ANNUAL is unset
	trialDays     int
	webhookSecret string
}

// NewClient builds a Stripe client from config. Returns an error if the secret
// key or price id is missing (billing cannot function without them). The annual
// price is optional: when unset it falls back to the monthly price so checkout
// still works before the annual price exists in the Stripe dashboard.
func NewClient(cfg Config) (*Client, error) {
	if cfg.SecretKey == "" {
		return nil, errors.New("stripe: STRIPE_SECRET_KEY is required")
	}
	if cfg.PriceID == "" {
		return nil, errors.New("stripe: STRIPE_PRICE_ID is required")
	}
	annual := cfg.PriceIDAnnual
	if annual == "" {
		annual = cfg.PriceID
	}
	return &Client{
		api:           stripe.NewClient(cfg.SecretKey),
		priceID:       cfg.PriceID,
		priceIDAnnual: annual,
		trialDays:     cfg.TrialDays,
		webhookSecret: cfg.WebhookSecret,
	}, nil
}

// priceFor selects the Stripe price for the requested cadence. Only "annual"
// selects the annual price; everything else (including "" and unknown values)
// falls back to monthly.
func (c *Client) priceFor(plan string) string {
	if plan == "annual" {
		return c.priceIDAnnual
	}
	return c.priceID
}

// WebhookSecret returns the configured Stripe webhook signing secret.
func (c *Client) WebhookSecret() string { return c.webhookSecret }

// CheckoutInput carries the per-request values for a Checkout Session.
type CheckoutInput struct {
	TenantID   string
	Email      string
	SuccessURL string
	CancelURL  string
	Plan       string // "monthly" (default) or "annual" — selects the price
}

// CreateCheckoutSession starts a subscription Checkout with the configured trial.
// The tenant id is stamped on both the session (ClientReferenceID) and the
// subscription metadata so every webhook event can be mapped back to a tenant.
func (c *Client) CreateCheckoutSession(ctx context.Context, in CheckoutInput) (url string, err error) {
	if in.TenantID == "" {
		return "", errors.New("checkout: tenant id required")
	}
	params := &stripe.CheckoutSessionCreateParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		ClientReferenceID: stripe.String(in.TenantID),
		SuccessURL:        stripe.String(in.SuccessURL),
		CancelURL:         stripe.String(in.CancelURL),
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Price:    stripe.String(c.priceFor(in.Plan)),
			Quantity: stripe.Int64(1),
		}},
		SubscriptionData: &stripe.CheckoutSessionCreateSubscriptionDataParams{
			TrialPeriodDays: stripe.Int64(int64(c.trialDays)),
			// Metadata on the subscription enables webhook self-heal (a
			// subscription.updated arriving before checkout.session.completed can
			// still find the tenant).
			Metadata: map[string]string{"tenant_id": in.TenantID},
		},
	}
	if in.Email != "" {
		params.CustomerEmail = stripe.String(in.Email)
	}
	sess, err := c.api.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("checkout: create session: %w", err)
	}
	return sess.URL, nil
}

// CreatePortalSession returns a Customer Portal URL where the tenant manages the
// subscription (cancel, update card, view invoices).
func (c *Client) CreatePortalSession(ctx context.Context, customerID, returnURL string) (url string, err error) {
	if customerID == "" {
		return "", errors.New("portal: customer id required")
	}
	sess, err := c.api.V1BillingPortalSessions.Create(ctx, &stripe.BillingPortalSessionCreateParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	})
	if err != nil {
		return "", fmt.Errorf("portal: create session: %w", err)
	}
	return sess.URL, nil
}

// GetSubscription fetches a subscription by id (for webhook self-heal: reading
// its metadata/customer when the local link is not yet written).
func (c *Client) GetSubscription(ctx context.Context, subID string) (*stripe.Subscription, error) {
	if subID == "" {
		return nil, errors.New("get subscription: id required")
	}
	sub, err := c.api.V1Subscriptions.Retrieve(ctx, subID, nil)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return sub, nil
}
