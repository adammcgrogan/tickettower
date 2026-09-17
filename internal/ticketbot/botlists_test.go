package ticketbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		if r.Header.Get("Authorization") == "bad" {
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
