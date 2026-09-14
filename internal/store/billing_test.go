package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSetGuildTierFromStripeAndBillingCustomer(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	seedGuild(t, s, 4001)

	if _, err := s.GuildBillingCustomer(ctx, 4001); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GuildBillingCustomer before subscribing = %v, want ErrNotFound", err)
	}

	future := time.Now().Add(30 * 24 * time.Hour)
	if err := s.SetGuildTierFromStripe(ctx, 4001, "premium", "sub_123", &future); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGuildBillingCustomer(ctx, 4001, "cus_123"); err != nil {
		t.Fatal(err)
	}

	tier, err := s.GuildTier(ctx, 4001)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "premium" {
		t.Errorf("tier = %q, want premium", tier)
	}
	cust, err := s.GuildBillingCustomer(ctx, 4001)
	if err != nil {
		t.Fatal(err)
	}
	if cust != "cus_123" {
		t.Errorf("customer = %q, want cus_123", cust)
	}

	// A subscription cancellation clears the entitlements row but keeps the
	// customer on file so "manage billing" and resubscribing still work.
	if err := s.SetGuildTierFromStripe(ctx, 4001, "free", "", nil); err != nil {
		t.Fatal(err)
	}
	tier, err = s.GuildTier(ctx, 4001)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "free" {
		t.Errorf("tier after cancel = %q, want free", tier)
	}
	cust, err = s.GuildBillingCustomer(ctx, 4001)
	if err != nil {
		t.Fatal(err)
	}
	if cust != "cus_123" {
		t.Errorf("customer after cancel = %q, want cus_123 (kept)", cust)
	}
}

func TestSetGuildTierFromStripeExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	seedGuild(t, s, 4002)

	past := time.Now().Add(-time.Hour)
	if err := s.SetGuildTierFromStripe(ctx, 4002, "premium", "sub_456", &past); err != nil {
		t.Fatal(err)
	}
	tier, err := s.GuildTier(ctx, 4002)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "free" {
		t.Errorf("tier with expired safety net = %q, want free", tier)
	}
}
