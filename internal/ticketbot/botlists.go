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
	// commandsURL, for sites that list slash commands, is where the bot's
	// command definitions are sent (the same JSON as registered with Discord).
	commandsURL func(botID snowflake.ID) string
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
			commandsURL: func(id snowflake.ID) string {
				return botListBase["discordbotlist.com"] + "/api/v1/bots/" + id.String() + "/commands"
			},
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
// limits. Sites that list commands get them once per startup, so a deploy
// that changes commands updates the listing.
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
	for _, l := range lists {
		if l.commandsURL == nil {
			continue
		}
		if err := postJSON(ctx, l.commandsURL(botID), l.token, commands); err != nil && ctx.Err() == nil {
			b.log.Warn("post command list", slog.String("site", l.name), slog.Any("err", err))
		}
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
	return postJSON(ctx, l.url(botID), l.token, l.body(servers))
}

// postJSON posts v to a bot list API. The sites' docs disagree on whether
// the token needs a "Bot " prefix, so a rejected bare token is retried with
// one.
func postJSON(ctx context.Context, url, token string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	status, msg, err := post(ctx, url, token, body)
	if err == nil && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		status, msg, err = post(ctx, url, "Bot "+token, body)
	}
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("%d: %s", status, msg)
	}
	return nil
}

func post(ctx context.Context, url, auth string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
	return res.StatusCode, bytes.TrimSpace(msg), nil
}
