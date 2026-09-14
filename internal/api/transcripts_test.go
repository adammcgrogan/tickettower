package api

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

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

func TestRenderTranscriptMarkdown(t *testing.T) {
	users := map[string]string{"1": "Ada"}
	roles := map[string]string{"2": "Moderators"}
	channels := map[string]string{"3": "billing"}
	cases := map[string]string{
		"<script>alert(1)</script>": "&lt;script&gt;alert(1)&lt;/script&gt;",
		"**bold** and _em_":         "<strong>bold</strong> and <em>em</em>",
		"<@1> ping":                 `<span class="md-mention">@Ada</span> ping`,
		"<@&2> ping":                `<span class="md-mention">@Moderators</span> ping`,
		"<#3> channel":              `<span class="md-mention">#billing</span>`,
		"<@999> ghost":              `unknown-user`,
		"`inline`":                  `<code class="md-code">inline</code>`,
		"see https://example.com":   `<a href="https://example.com" target="_blank" rel="noopener noreferrer nofollow" class="md-link">https://example.com</a>`,
	}
	for input, want := range cases {
		got := renderTranscriptMarkdown(input, users, roles, channels)
		if !strings.Contains(got, want) {
			t.Errorf("renderTranscriptMarkdown(%q) = %q, want to contain %q", input, got, want)
		}
	}
}

func TestRenderTranscriptHTML(t *testing.T) {
	opened := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	data := transcriptData{
		Ticket: store.Ticket{
			Number:     7,
			TypeName:   "Support",
			OpenerID:   snowflake.ID(1),
			OpenerName: "Ada",
			Status:     store.StatusOpen,
			OpenedAt:   opened,
		},
		Messages: []store.TicketMessage{
			{
				ID: snowflake.ID(1), AuthorID: snowflake.ID(1), AuthorName: "Ada",
				Content: "Hello <script>evil()</script> **team**", CreatedAt: opened,
				Attachments: []store.Attachment{{Name: "log.txt", URL: "https://cdn.discordapp.com/x.txt", Size: 2048}},
			},
		},
		Guild: transcriptGuild{Name: "Acme Support"},
	}

	var buf bytes.Buffer
	if err := renderTranscriptHTML(&buf, data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"Ticket #7", "Acme Support", "Ada",
		"<strong>team</strong>",
		"log.txt", "2 KB",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "<script>evil()</script>") {
		t.Error("message content wasn't escaped")
	}
}
