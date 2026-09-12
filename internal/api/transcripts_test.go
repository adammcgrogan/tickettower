package api

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func signedURL(name string, expires time.Time) string {
	return fmt.Sprintf("https://cdn.discordapp.com/attachments/1/2/%s?ex=%x&is=0&hm=sig", name, expires.Unix())
}

func TestRenewAttachmentLinks(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	expired := signedURL("old.png", now.Add(-2*time.Hour))
	soon := signedURL("soon.png", now.Add(10*time.Minute))
	fine := signedURL("fine.png", now.Add(6*time.Hour))
	unsigned := "https://cdn.discordapp.com/attachments/1/2/plain.png"
	messages := []store.TicketMessage{
		{Attachments: []store.Attachment{{URL: expired}, {URL: fine}}},
		{Attachments: []store.Attachment{{URL: soon}, {URL: unsigned}, {URL: expired}}},
	}

	var asked [][]string
	newExpiry := now.Add(24 * time.Hour)
	refresh := func(urls []string) (map[string]string, error) {
		asked = append(asked, slices.Clone(urls))
		return map[string]string{expired: signedURL("old.png", newExpiry)}, nil // Discord didn't renew soon.png
	}
	cache := newTTLCache()
	if err := renewAttachmentLinks(messages, now, cache, refresh); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !slices.Equal(asked[0], []string{expired, soon}) {
		t.Errorf("asked to refresh %v, want [expired soon] once", asked)
	}
	if got := messages[0].Attachments[0].URL; got != signedURL("old.png", newExpiry) {
		t.Errorf("expired link = %s, want renewed", got)
	}
	if messages[1].Attachments[2].URL != messages[0].Attachments[0].URL {
		t.Error("the same link in another message wasn't renewed")
	}
	for _, want := range []string{fine, soon, unsigned} {
		found := slices.ContainsFunc(messages, func(m store.TicketMessage) bool {
			return slices.ContainsFunc(m.Attachments, func(a store.Attachment) bool { return a.URL == want })
		})
		if !found {
			t.Errorf("link %s should be untouched", want)
		}
	}

	// A second view uses the cache instead of asking Discord again.
	messages[0].Attachments[0].URL = expired
	asked = nil
	refresh = func(urls []string) (map[string]string, error) {
		asked = append(asked, urls)
		return nil, errors.New("discord down")
	}
	err := renewAttachmentLinks(messages[:1], now, cache, refresh)
	if err != nil || len(asked) != 0 || messages[0].Attachments[0].URL != signedURL("old.png", newExpiry) {
		t.Errorf("cached renewal: err=%v asked=%v url=%s", err, asked, messages[0].Attachments[0].URL)
	}

	// A failed refresh keeps the stored links and reports the error.
	messages[1].Attachments[0].URL = soon
	if err := renewAttachmentLinks(messages[1:], now, cache, refresh); err == nil {
		t.Error("refresh failure not reported")
	}
	if messages[1].Attachments[0].URL != soon {
		t.Error("link changed despite a failed refresh")
	}
}
