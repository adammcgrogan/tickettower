package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// SetGuildTierFromStripe sets or clears a guild's plan from a Stripe
// subscription event. Like SetGuildTier, "free" removes the entitlements
// row rather than writing one. expiresAt is a safety-net cutoff (the
// subscription's current period end) in case a later webhook is ever
// missed; GuildTier already treats an expired row as absent.
func (s *Store) SetGuildTierFromStripe(ctx context.Context, guildID snowflake.ID, tier, subscriptionID string, expiresAt *time.Time) error {
	if tier == "free" {
		_, err := s.pool.Exec(ctx, `DELETE FROM entitlements WHERE guild_id = $1`, int64(guildID))
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO entitlements (guild_id, tier, source, stripe_subscription_id, expires_at)
		VALUES ($1, $2, 'stripe', $3, $4)
		ON CONFLICT (guild_id) DO UPDATE SET
			tier = EXCLUDED.tier, source = 'stripe',
			stripe_subscription_id = EXCLUDED.stripe_subscription_id, expires_at = EXCLUDED.expires_at`,
		int64(guildID), tier, subscriptionID, expiresAt)
	return err
}

// UpsertGuildBillingCustomer records the Stripe customer a guild has bought
// (or is buying) a subscription through, so future checkouts reuse it and
// the billing portal has something to open.
func (s *Store) UpsertGuildBillingCustomer(ctx context.Context, guildID snowflake.ID, customerID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO guild_billing (guild_id, stripe_customer_id) VALUES ($1, $2)
		ON CONFLICT (guild_id) DO UPDATE SET stripe_customer_id = EXCLUDED.stripe_customer_id`,
		int64(guildID), customerID)
	return err
}

// GuildBillingCustomer returns the Stripe customer ID a guild has on file,
// or ErrNotFound if it has never started a subscription.
func (s *Store) GuildBillingCustomer(ctx context.Context, guildID snowflake.ID) (string, error) {
	var customerID string
	err := s.pool.QueryRow(ctx, `SELECT stripe_customer_id FROM guild_billing WHERE guild_id = $1`, int64(guildID)).
		Scan(&customerID)
	return customerID, notFound(err)
}
