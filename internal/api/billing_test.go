package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// billingTestEnv wires a real Postgres-backed Server (billing needs the
// store; nothing else in these tests touches Discord or Redis) with a fake
// Stripe client so no network call ever happens.
type billingTestEnv struct {
	srv    *Server
	store  *store.Store
	stripe *fakeStripeClient
}

func newBillingTestEnv(t *testing.T) billingTestEnv {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		PublicURL:           "http://localhost:5173",
		StripeWebhookSecret: "whsec_test",
		StripePriceID:       "price_test",
	}
	srv := NewServer(cfg, st, nil, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	fake := &fakeStripeClient{url: "https://checkout.stripe.com/test-session"}
	srv.stripe = fake
	return billingTestEnv{srv: srv, store: st, stripe: fake}
}

// billingTestGuild returns a snowflake ID unlikely to collide with fixtures
// used by internal/store's own tests against the same database, and seeds a
// guild row for it.
func billingTestGuild(t *testing.T, e billingTestEnv, n int64) snowflake.ID {
	t.Helper()
	id := snowflake.ID(90_000_000_000_000_000 + n)
	if err := e.store.UpsertGuild(context.Background(), store.Guild{ID: id, Name: "Test", OwnerID: 1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.store.DeleteGuildData(context.Background(), id) //nolint:errcheck
	})
	return id
}

type fakeStripeClient struct {
	url              string
	err              error
	checkoutGuildID  snowflake.ID
	checkoutCustomer string
	portalCustomer   string
	checkoutCalls    int
	portalCalls      int
}

func (f *fakeStripeClient) NewCheckoutSession(_ context.Context, guildID snowflake.ID, customerID string) (string, error) {
	f.checkoutCalls++
	f.checkoutGuildID = guildID
	f.checkoutCustomer = customerID
	return f.url, f.err
}

func (f *fakeStripeClient) NewPortalSession(_ context.Context, customerID string) (string, error) {
	f.portalCalls++
	f.portalCustomer = customerID
	return f.url, f.err
}

// requestAsGuild builds a request whose context already carries the access
// a guild admin's request would have after passing through requireGuild and
// requireLevel(LevelAdmin) — those middleware are exercised elsewhere via
// the OAuth/session machinery; here we're testing the handlers themselves.
func requestAsGuild(guildID snowflake.ID) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	acc := guildAccess{guild: discord.OAuth2Guild{ID: guildID}, level: store.LevelAdmin}
	return req.WithContext(context.WithValue(req.Context(), guildCtxKey{}, acc))
}

func TestCreateCheckoutSessionReusesStoredCustomer(t *testing.T) {
	e := newBillingTestEnv(t)
	guildID := billingTestGuild(t, e, 1)
	if err := e.store.UpsertGuildBillingCustomer(context.Background(), guildID, "cus_existing"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	e.srv.createCheckoutSession(rec, requestAsGuild(guildID))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct{ URL string }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.URL != e.stripe.url {
		t.Errorf("url = %q", resp.URL)
	}
	// Whoever gets billed and which existing customer is reused come only
	// from the authenticated guild in context and the store — never from
	// request input (this handler reads no body).
	if e.stripe.checkoutGuildID != guildID {
		t.Errorf("checkout guild = %v, want %v", e.stripe.checkoutGuildID, guildID)
	}
	if e.stripe.checkoutCustomer != "cus_existing" {
		t.Errorf("checkout customer = %q, want cus_existing (reused)", e.stripe.checkoutCustomer)
	}
}

func TestCreateCheckoutSessionNewCustomer(t *testing.T) {
	e := newBillingTestEnv(t)
	guildID := billingTestGuild(t, e, 2)

	rec := httptest.NewRecorder()
	e.srv.createCheckoutSession(rec, requestAsGuild(guildID))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if e.stripe.checkoutCustomer != "" {
		t.Errorf("checkout customer = %q, want empty (let Stripe create one)", e.stripe.checkoutCustomer)
	}
}

func TestCreatePortalSessionRequiresExistingCustomer(t *testing.T) {
	e := newBillingTestEnv(t)
	guildID := billingTestGuild(t, e, 3)

	// No subscription on file yet.
	rec := httptest.NewRecorder()
	e.srv.createPortalSession(rec, requestAsGuild(guildID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404: %s", rec.Code, rec.Body.String())
	}

	if err := e.store.UpsertGuildBillingCustomer(context.Background(), guildID, "cus_abc"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	e.srv.createPortalSession(rec, requestAsGuild(guildID))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if e.stripe.portalCustomer != "cus_abc" {
		t.Errorf("portal customer = %q, want cus_abc", e.stripe.portalCustomer)
	}
}

// --- webhook ---

func subscriptionEventBody(t *testing.T, guildID snowflake.ID, status, subID, customerID string) []byte {
	t.Helper()
	payload := map[string]any{
		"id":     "evt_test",
		"object": "event",
		"type":   "customer.subscription.updated",
		"data": map[string]any{
			"object": map[string]any{
				"id":       subID,
				"object":   "subscription",
				"status":   status,
				"customer": customerID,
				"metadata": map[string]string{"guild_id": guildID.String()},
				"items": map[string]any{
					"object": "list",
					"data": []map[string]any{{
						"id": "si_1", "object": "subscription_item", "current_period_end": 4102444800,
					}},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func postWebhook(t *testing.T, e billingTestEnv, body []byte, secret string) *httptest.ResponseRecorder {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: secret})
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", signed.Header)
	rec := httptest.NewRecorder()
	e.srv.stripeWebhook(rec, req)
	return rec
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	e := newBillingTestEnv(t)
	guildID := billingTestGuild(t, e, 4)
	body := subscriptionEventBody(t, guildID, "active", "sub_bad", "cus_bad")

	rec := postWebhook(t, e, body, "wrong-secret")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	tier, err := e.store.GuildTier(context.Background(), guildID)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "free" {
		t.Errorf("tier after forged webhook = %q, want free (untouched)", tier)
	}
}

func TestWebhookGrantsAndRevokesPremium(t *testing.T) {
	e := newBillingTestEnv(t)
	guildID := billingTestGuild(t, e, 5)

	active := subscriptionEventBody(t, guildID, "active", "sub_1", "cus_1")
	rec := postWebhook(t, e, active, e.srv.cfg.StripeWebhookSecret)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	tier, err := e.store.GuildTier(context.Background(), guildID)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "premium" {
		t.Fatalf("tier = %q, want premium", tier)
	}
	cust, err := e.store.GuildBillingCustomer(context.Background(), guildID)
	if err != nil || cust != "cus_1" {
		t.Fatalf("customer = %q, %v", cust, err)
	}

	// Replaying the same event is a no-op, not an error.
	rec = postWebhook(t, e, active, e.srv.cfg.StripeWebhookSecret)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay: got %d", rec.Code)
	}

	canceled := subscriptionEventBody(t, guildID, "canceled", "sub_1", "cus_1")
	rec = postWebhook(t, e, canceled, e.srv.cfg.StripeWebhookSecret)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	tier, err = e.store.GuildTier(context.Background(), guildID)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "free" {
		t.Errorf("tier after cancellation = %q, want free", tier)
	}
	// The customer stays on file so "manage billing" and a resubscribe
	// still work.
	if cust, err := e.store.GuildBillingCustomer(context.Background(), guildID); err != nil || cust != "cus_1" {
		t.Errorf("customer after cancellation = %q, %v, want kept", cust, err)
	}
}

func TestWebhookIgnoresEventWithoutGuildMetadata(t *testing.T) {
	e := newBillingTestEnv(t)
	body := []byte(fmt.Sprintf(`{
		"id": "evt_test2", "object": "event", "type": "customer.subscription.updated",
		"data": {"object": {"id": "sub_x", "object": "subscription", "status": "active", "customer": "cus_x", "metadata": {}}}
	}`))
	rec := postWebhook(t, e, body, e.srv.cfg.StripeWebhookSecret)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (acknowledged, ignored)", rec.Code)
	}
}
