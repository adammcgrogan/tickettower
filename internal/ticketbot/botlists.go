package ticketbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// botList is a bot listing site the bot reports its server count to, so the
// listing shows a live number.
type botList struct {
	name  string
	token string
	// url is the stats endpoint for a bot ID.
	url func(botID snowflake.ID) string
	// body is the JSON each site expects for a server count.
	body func(servers int) any
}

// botListBase is overridden in tests.
var botListBase = map[string]string{
	"top.gg":             "https://top.gg",
	"discordbotlist.com": "https://discordbotlist.com",
	"discord.bots.gg":    "https://discord.bots.gg",
}

// botLists returns the listing sites with a token configured.
func (b *Bot) botLists() []botList {
	all := []botList{
		{
			name:  "top.gg",
			token: b.cfg.TopggToken,
			url:   func(id snowflake.ID) string { return botListBase["top.gg"] + "/api/bots/" + id.String() + "/stats" },
			body:  func(n int) any { return map[string]int{"server_count": n} },
		},
		{
			name:  "discordbotlist.com",
			token: b.cfg.DiscordBotListToken,
			url: func(id snowflake.ID) string {
				return botListBase["discordbotlist.com"] + "/api/v1/bots/" + id.String() + "/stats"
			},
			body: func(n int) any { return map[string]int{"guilds": n} },
		},
		{
			name:  "discord.bots.gg",
			token: b.cfg.DiscordBotsGGToken,
			url: func(id snowflake.ID) string {
				return botListBase["discord.bots.gg"] + "/api/v1/bots/" + id.String() + "/stats"
			},
			body: func(n int) any { return map[string]int{"guildCount": n} },
		},
	}
	var lists []botList
	for _, l := range all {
		if l.token != "" {
			lists = append(lists, l)
		}
	}
	return lists
}

// postServerCounts reports the server count to every configured listing site
// shortly after startup and then every 30 minutes, well inside their rate
// limits.
func (b *Bot) postServerCounts(ctx context.Context, botID snowflake.ID) {
	lists := b.botLists()
	if len(lists) == 0 {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Minute): // let guilds finish loading
	}
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		b.postServerCountsOnce(ctx, botID, lists)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *Bot) postServerCountsOnce(ctx context.Context, botID snowflake.ID, lists []botList) {
	n, err := b.store.ActiveGuildCount(ctx)
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("count servers for bot lists", slog.Any("err", err))
		}
		return
	}
	for _, l := range lists {
		if err := postServerCount(ctx, l, botID, n); err != nil && ctx.Err() == nil {
			b.log.Warn("post server count", slog.String("site", l.name), slog.Any("err", err))
		}
	}
}

func postServerCount(ctx context.Context, l botList, botID snowflake.ID, servers int) error {
	body, err := json.Marshal(l.body(servers))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.url(botID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", l.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s: %s", res.Status, bytes.TrimSpace(msg))
	}
	return nil
}
