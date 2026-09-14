-- +goose Up
ALTER TABLE entitlements ADD COLUMN stripe_subscription_id TEXT;

-- One row per guild that has ever started a Stripe subscription, so a
-- lapsed guild's "Manage billing" link and resubscribe both keep working
-- even after entitlements.tier = free deletes its entitlements row.
CREATE TABLE guild_billing (
    guild_id           BIGINT PRIMARY KEY REFERENCES guilds (id) ON DELETE CASCADE,
    stripe_customer_id TEXT        NOT NULL UNIQUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE guild_billing;
ALTER TABLE entitlements DROP COLUMN stripe_subscription_id;
