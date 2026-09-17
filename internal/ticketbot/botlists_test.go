package ticketbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/adammcgrogan/tickettower/internal/config"
)

func TestPostServerCount(t *testing.T) {
	type hit struct {
		path, auth string
		body       map[string]int
	}
	var mu sync.Mutex
	var hits []hit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]int
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		hits = append(hits, hit{r.URL.Path, r.Header.Get("Authorization"), body})
		mu.Unlock()
		if strings.HasSuffix(r.Header.Get("Authorization"), "bad") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	old := botListBase
	botListBase = map[string]string{"top.gg": srv.URL, "discordbotlist.com": srv.URL, "discord.bots.gg": srv.URL}
	t.Cleanup(func() { botListBase = old })

	b := &Bot{cfg: config.Config{TopggToken: "tg", DiscordBotListToken: "dbl"}}
	lists := b.botLists()
	if len(lists) != 2 {
		t.Fatalf("lists = %d, want only the two with tokens", len(lists))
	}
	for _, l := range lists {
		if err := postServerCount(context.Background(), l, 42, 7); err != nil {
			t.Fatalf("%s: %v", l.name, err)
		}
	}
	want := []hit{
		{"/api/bots/42/stats", "tg", map[string]int{"server_count": 7}},
		{"/api/v1/bots/42/stats", "dbl", map[string]int{"guilds": 7}},
	}
	for i, w := range want {
		got := hits[i]
		if got.path != w.path || got.auth != w.auth || len(got.body) != 1 {
			t.Errorf("hit %d = %+v, want %+v", i, got, w)
		}
		for k, v := range w.body {
			if got.body[k] != v {
				t.Errorf("hit %d body = %v, want %v", i, got.body, w.body)
			}
		}
	}

	lists[0].token = "bad"
	if err := postServerCount(context.Background(), lists[0], 42, 7); err == nil {
		t.Error("a rejected token should be an error")
	}
}

func TestPostCommandList(t *testing.T) {
	var auths []string
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		// Only the "Bot " form is accepted, to exercise the retry.
		if r.Header.Get("Authorization") != "Bot dbl" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/bots/42/commands" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	old := botListBase
	botListBase = map[string]string{"top.gg": srv.URL, "discordbotlist.com": srv.URL, "discord.bots.gg": srv.URL}
	t.Cleanup(func() { botListBase = old })

	b := &Bot{cfg: config.Config{DiscordBotListToken: "dbl"}}
	l := b.botLists()[0]
	if err := postJSON(context.Background(), l.commandsURL(42), l.token, commands); err != nil {
		t.Fatal(err)
	}
	if len(auths) != 2 || auths[0] != "dbl" {
		t.Errorf("auth attempts = %v, want bare then Bot-prefixed", auths)
	}
	names := map[string]bool{}
	for _, c := range got {
		names[c["name"].(string)] = true
		if c["type"] != float64(1) || c["description"] == "" {
			t.Errorf("command = %v, want a type 1 slash command with a description", c)
		}
	}
	for _, want := range []string{"help", "ticket", "close", "reply", "ping"} {
		if !names[want] {
			t.Errorf("command list is missing /%s: %v", want, names)
		}
	}
}
