package ticketbot

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"

	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// weeklySummaryCheckInterval is how often the bot looks for a guild's
// weekly summary being due; a guild only actually gets one every
// store.WeeklySummaryInterval.
const weeklySummaryCheckInterval = time.Hour

// postWeeklySummaries posts each opted-in premium guild's weekly summary to
// its log channel, checking once at startup and then hourly.
func (b *Bot) postWeeklySummaries(ctx context.Context) {
	ticker := time.NewTicker(weeklySummaryCheckInterval)
	defer ticker.Stop()
	for {
		b.runWeeklySummaries(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *Bot) runWeeklySummaries(ctx context.Context) {
	now := time.Now()
	due, err := b.store.GuildsDueWeeklySummary(ctx, now)
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("failed to find guilds due a weekly summary", slog.Any("err", err))
		}
		return
	}
	for _, g := range due {
		// Recorded before it's posted, so a restart or a slow Discord
		// response never sends the same guild two summaries.
		if err := b.store.MarkWeeklySummarySent(ctx, g.GuildID, now); err != nil {
			b.log.Error("failed to record weekly summary sent", slog.String("guild_id", g.GuildID.String()), slog.Any("err", err))
			continue
		}
		b.postWeeklySummary(ctx, g)
	}
}

func (b *Bot) postWeeklySummary(ctx context.Context, g store.WeeklySummaryTarget) {
	a, err := b.store.Analytics(ctx, g.GuildID, store.AnalyticsQuery{Days: 7})
	if err != nil {
		b.log.Error("failed to load weekly summary analytics", slog.String("guild_id", g.GuildID.String()), slog.Any("err", err))
		return
	}
	_, err = b.rest.CreateMessage(g.LogChannelID, weeklySummaryMessage(a), rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel, discordx.CodeMissingAccess) {
		b.log.Warn("failed to post weekly summary", slog.String("guild_id", g.GuildID.String()), slog.Any("err", err))
	}
}

// weeklySummaryMessage builds the recap embed: tickets opened and closed,
// median first response, satisfaction and the busiest day, each for the
// last 7 days compared with the week before.
func weeklySummaryMessage(a store.Analytics) discord.MessageCreate {
	s := a.Summary
	var prevOpened, prevClosed int
	if a.Previous != nil {
		prevOpened, prevClosed = a.Previous.Opened, a.Previous.Closed
	}
	embed := discord.NewEmbed().
		WithTitle("This week in support").
		WithDescription("The last 7 days, compared with the week before.").
		WithColor(colorAccent).
		AddField("Opened", fmt.Sprintf("%d%s", s.Opened, weekOverWeek(s.Opened, prevOpened)), true).
		AddField("Closed", fmt.Sprintf("%d%s", s.Closed, weekOverWeek(s.Closed, prevClosed)), true).
		AddField("Busiest day", cmp.Or(busiestDay(a.Series), "—"), true).
		AddField("Median first response", medianLabel(s.FirstResponseMedianSec), true).
		AddField("Satisfaction", satisfactionLabel(s.RatingAvg, s.RatingCount), true).
		WithTimestamp(time.Now())
	return discord.NewMessageCreate().WithEmbeds(embed).WithAllowedMentions(&discord.AllowedMentions{})
}

// weekOverWeek notes the change from the previous week, if there was one to
// compare against.
func weekOverWeek(cur, prev int) string {
	switch d := cur - prev; {
	case prev == 0 && cur == 0:
		return ""
	case d > 0:
		return fmt.Sprintf(" (+%d)", d)
	case d < 0:
		return fmt.Sprintf(" (%d)", d)
	default:
		return " (same as last week)"
	}
}

// medianLabel formats a median duration in seconds, or a dash if nothing
// was measured.
func medianLabel(sec *float64) string {
	if sec == nil {
		return "—"
	}
	return humanDuration(time.Duration(*sec * float64(time.Second)))
}

// satisfactionLabel summarises the opener ratings left in the window.
func satisfactionLabel(avg *float64, count int) string {
	if avg == nil || count == 0 {
		return "No ratings yet"
	}
	unit := "rating"
	if count != 1 {
		unit = "ratings"
	}
	return fmt.Sprintf("%.1f/5 (%d %s)", *avg, count, unit)
}

// busiestDay returns the weekday with the most activity (tickets opened or
// closed) in series, or "" if there was none.
func busiestDay(series []store.SeriesPoint) string {
	best, bestActivity := "", 0
	for _, p := range series {
		if activity := p.Opened + p.Closed; activity > bestActivity {
			if d, err := time.Parse(time.DateOnly, p.Date); err == nil {
				best, bestActivity = d.Weekday().String(), activity
			}
		}
	}
	return best
}
