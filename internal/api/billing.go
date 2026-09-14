package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// createCheckoutSession starts a Stripe Checkout subscription for the
// guild, reusing its existing Stripe customer if it has one.
func (s *Server) createCheckoutSession(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	customerID, err := s.store.GuildBillingCustomer(r.Context(), g.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.writeFailure(w, err)
		return
	}
	url, err := s.stripe.NewCheckoutSession(r.Context(), g.ID, customerID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// createPortalSession opens Stripe's self-serve billing portal for a guild
// that has bought a subscription before.
func (s *Server) createPortalSession(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	customerID, err := s.store.GuildBillingCustomer(r.Context(), g.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "this server hasn't subscribed yet")
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	url, err := s.stripe.NewPortalSession(r.Context(), customerID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// stripeWebhookBodyLimit caps how much of a webhook request body is read,
// well above any real Stripe event, so a malformed or abusive request can't
// exhaust memory.
const stripeWebhookBodyLimit = 256 << 10

// stripeWebhook is the only route in the API that isn't behind a session
// cookie, since Stripe (not a browser) calls it. It verifies Stripe's
// signature before touching anything, trusts nothing but the verified
// event body, and its only side effect is an idempotent tier upsert or
// clear — nothing from the request is ever reflected back in the response.
func (s *Server) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, stripeWebhookBodyLimit))
	if err != nil {
		writeError(w, http.StatusBadRequest, "request body too large")
		return
	}
	// IgnoreAPIVersionMismatch: the webhook endpoint's configured API
	// version (set in the Stripe dashboard) won't always match the exact
	// version stripe-go is pinned to, and we only read a handful of fields
	// that have been stable for years — the signature check below is what
	// actually matters for security.
	event, err := webhook.ConstructEventWithOptions(body, r.Header.Get("Stripe-Signature"), s.cfg.StripeWebhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid signature")
		return
	}

	switch event.Type {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		s.handleSubscriptionEvent(r.Context(), event)
	default:
		// Not an event we act on; acknowledge so Stripe stops retrying it.
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSubscriptionEvent(ctx context.Context, event stripe.Event) {
	var sub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		s.log.Error("decode stripe subscription event", slog.Any("err", err))
		return
	}
	guildID, err := snowflake.Parse(sub.Metadata["guild_id"])
	if err != nil {
		// Not one of our subscriptions (or predates metadata being set);
		// nothing we can attribute this to.
		s.log.Warn("stripe subscription event missing guild_id metadata", slog.String("subscription", sub.ID))
		return
	}

	tier := "free"
	var expiresAt *time.Time
	if sub.Status == stripe.SubscriptionStatusActive || sub.Status == stripe.SubscriptionStatusTrialing {
		tier = "premium"
		if sub.Items != nil && len(sub.Items.Data) > 0 {
			t := time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0)
			expiresAt = &t
		}
	}
	if err := s.store.SetGuildTierFromStripe(ctx, guildID, tier, sub.ID, expiresAt); err != nil {
		s.log.Error("set guild tier from stripe", slog.Any("err", err))
		return
	}
	if sub.Customer != nil && sub.Customer.ID != "" {
		if err := s.store.UpsertGuildBillingCustomer(ctx, guildID, sub.Customer.ID); err != nil {
			s.log.Error("upsert guild billing customer", slog.Any("err", err))
		}
	}
}
