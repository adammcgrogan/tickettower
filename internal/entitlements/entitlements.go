// Package entitlements defines per-plan limits. A guild is premium when it
// has a row in the entitlements table with tier = "premium" (see
// store.GuildTier); there's no billing wired up yet, so that row is set by
// hand until issue #77's payment work lands.
package entitlements

const (
	TierFree    = "free"
	TierPremium = "premium"
)

type Limits struct {
	MaxPanels       int `json:"max_panels"`
	MaxTicketTypes  int `json:"max_ticket_types"`
	MaxSavedReplies int `json:"max_saved_replies"`
	// MaxTranscriptRetentionDays caps how long transcripts can be kept, and
	// disallows "keep forever". 0 means no cap.
	MaxTranscriptRetentionDays int `json:"max_transcript_retention_days"`
	// Branding is true when a "Powered by" footer belongs on the guild's
	// panel messages.
	Branding bool `json:"branding"`
}

func ForTier(tier string) Limits {
	switch tier {
	case TierPremium:
		return Limits{MaxPanels: 50, MaxTicketTypes: 100, MaxSavedReplies: 200}
	default:
		return Limits{MaxPanels: 10, MaxTicketTypes: 25, MaxSavedReplies: 50, MaxTranscriptRetentionDays: 90, Branding: true}
	}
}
