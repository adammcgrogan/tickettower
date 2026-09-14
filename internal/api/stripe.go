package api

import (
	"context"
	"fmt"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stripe/stripe-go/v82"

	"github.com/adammcgrogan/tickettower/internal/config"
)

// stripeClient is the seam between the API's billing handlers and Stripe,
// so handler tests can inject a fake instead of hitting the real API.
type stripeClient interface {
	// NewCheckoutSession starts a subscription checkout for guildID,
	// reusing customerID if the guild has bought a subscription before
	// (empty string lets Stripe create a new customer).
	NewCheckoutSession(ctx context.Context, guildID snowflake.ID, customerID string) (url string, err error)
	// NewPortalSession opens Stripe's self-serve billing portal for an
	// existing customer.
	NewPortalSession(ctx context.Context, customerID string) (url string, err error)
}

type liveStripeClient struct {
	client *stripe.Client
	cfg    config.Config
}

func newStripeClient(cfg config.Config) stripeClient {
	return &liveStripeClient{client: stripe.NewClient(cfg.StripeSecretKey), cfg: cfg}
}

func (c *liveStripeClient) NewCheckoutSession(ctx context.Context, guildID snowflake.ID, customerID string) (string, error) {
	guild := guildID.String()
	params := &stripe.CheckoutSessionCreateParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems:  []*stripe.CheckoutSessionCreateLineItemParams{{Price: stripe.String(c.cfg.StripePriceID), Quantity: stripe.Int64(1)}},
		SuccessURL: stripe.String(c.cfg.PublicURL + "/servers/" + guild + "/settings?billing=success"),
		CancelURL:  stripe.String(c.cfg.PublicURL + "/servers/" + guild + "/settings?billing=cancel"),
		// ClientReferenceID and the subscription's own metadata both carry
		// the guild ID: the webhook only ever trusts the subscription's
		// metadata (set here, never client-controlled) to know which guild
		// an event belongs to.
		ClientReferenceID: stripe.String(guild),
		SubscriptionData: &stripe.CheckoutSessionCreateSubscriptionDataParams{
			Metadata: map[string]string{"guild_id": guild},
		},
	}
	if customerID != "" {
		params.Customer = stripe.String(customerID)
	}
	sess, err := c.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}
	return sess.URL, nil
}

func (c *liveStripeClient) NewPortalSession(ctx context.Context, customerID string) (string, error) {
	params := &stripe.BillingPortalSessionCreateParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(c.cfg.PublicURL),
	}
	sess, err := c.client.V1BillingPortalSessions.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create billing portal session: %w", err)
	}
	return sess.URL, nil
}
