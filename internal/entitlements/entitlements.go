// Package entitlements defines per-plan limits. Everything is free today;
// premium tiers only need a new case here and a row in the entitlements
// table.
package entitlements

const (
	TierFree    = "free"
	TierPremium = "premium"
)

type Limits struct {
	MaxPanels       int `json:"max_panels"`
	MaxTicketTypes  int `json:"max_ticket_types"`
	MaxSavedReplies int `json:"max_saved_replies"`
}

func ForTier(tier string) Limits {
	switch tier {
	case TierPremium:
		return Limits{MaxPanels: 50, MaxTicketTypes: 100, MaxSavedReplies: 200}
	default:
		return Limits{MaxPanels: 10, MaxTicketTypes: 25, MaxSavedReplies: 50}
	}
}
