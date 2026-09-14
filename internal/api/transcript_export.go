package api

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// renderTranscriptHTML writes a ticket transcript as a single, self-contained
// HTML file: inline styles, no external scripts, so it keeps working after
// the dashboard link stops (moderation appeals, handing a case to another
// platform, keeping a record past the retention period). Avatars and
// attachments stay as links to Discord's CDN rather than embedded data, since
// those links usually outlive the retention period even if not forever.
func renderTranscriptHTML(w io.Writer, data transcriptData) error {
	users := map[string]string{data.Ticket.OpenerID.String(): data.Ticket.OpenerName}
	for _, m := range data.Messages {
		users[m.AuthorID.String()] = m.AuthorName
	}
	v := transcriptView{
		Guild:    data.Guild,
		Ticket:   data.Ticket,
		Items:    groupTranscriptItems(data.Messages, data.Notes),
		users:    users,
		roles:    data.Roles,
		channels: data.Channels,
	}
	return transcriptTemplate.Execute(w, v)
}

type transcriptView struct {
	Guild    transcriptGuild
	Ticket   store.Ticket
	Items    []transcriptItem
	users    map[string]string
	roles    map[string]string
	channels map[string]string
}

// transcriptItem is either a run of consecutive messages from one author or
// a private staff note, in the order the transcript page shows them.
type transcriptItem struct {
	Note     *store.TicketNote
	Messages []store.TicketMessage
}

// groupTranscriptItems mirrors TranscriptView.svelte: notes are interleaved
// with messages by time, and consecutive messages from the same author
// within 7 minutes of each other are grouped under one heading.
func groupTranscriptItems(messages []store.TicketMessage, notes []store.TicketNote) []transcriptItem {
	sorted := append([]store.TicketNote(nil), notes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })

	var out []transcriptItem
	n := 0
	for _, msg := range messages {
		for n < len(sorted) && !sorted[n].CreatedAt.After(msg.CreatedAt) {
			out = append(out, transcriptItem{Note: &sorted[n]})
			n++
		}
		var prev *store.TicketMessage
		if len(out) > 0 && out[len(out)-1].Note == nil {
			last := out[len(out)-1].Messages
			prev = &last[len(last)-1]
		}
		sameAuthor := prev != nil && prev.AuthorID == msg.AuthorID
		recent := prev != nil && msg.CreatedAt.Sub(prev.CreatedAt) < 7*time.Minute
		if sameAuthor && recent {
			last := &out[len(out)-1]
			last.Messages = append(last.Messages, msg)
		} else {
			out = append(out, transcriptItem{Messages: []store.TicketMessage{msg}})
		}
	}
	for n < len(sorted) {
		out = append(out, transcriptItem{Note: &sorted[n]})
		n++
	}
	return out
}

// --- markdown ---
//
// A Go port of web/src/lib/markdown.ts, for the download file to look like
// the dashboard's transcript view without a JS runtime. It covers the same
// syntax but, for lack of RE2 lookaround, is a little more forgiving about
// where emphasis markers can start and end.

var (
	reFence       = regexp.MustCompile("(?s)```(?:[\\w+-]*\n)?(.*?)```")
	reInline      = regexp.MustCompile("`([^`\n]+)`")
	reUserMent    = regexp.MustCompile(`&lt;@!?(\d+)&gt;`)
	reRoleMent    = regexp.MustCompile(`&lt;@&amp;(\d+)&gt;`)
	reChanMent    = regexp.MustCompile(`&lt;#(\d+)&gt;`)
	reEmoji       = regexp.MustCompile(`&lt;a?:(\w+):\d+&gt;`)
	reTime        = regexp.MustCompile(`&lt;t:(\d+)(?::[a-zA-Z])?&gt;`)
	reMdLink      = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)]+)\)`)
	reBareURL     = regexp.MustCompile(`(^|[\s(])(https?://\S+)`)
	reBold        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reUnderln     = regexp.MustCompile(`__(.+?)__`)
	reItalicS     = regexp.MustCompile(`\*(\S(?:.*?\S)?|\S)\*`)
	reItalicU     = regexp.MustCompile(`_(\S(?:.*?\S)?|\S)_`)
	reStrike      = regexp.MustCompile(`~~(.+?)~~`)
	reSpoiler     = regexp.MustCompile(`\|\|(.+?)\|\|`)
	reHeading     = regexp.MustCompile(`^(#{1,3}) (.+)$`)
	reQuote       = regexp.MustCompile(`^&gt; ?(.*)$`)
	rePlaceholder = regexp.MustCompile(`\x00(\d+)\x00`)
)

func mdLink(url, text string) string {
	return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer nofollow" class="md-link">%s</a>`, url, text)
}

// renderTranscriptMarkdown renders one message/note/embed field's text the
// way Discord shows it. The input is untrusted (ticket content); every
// output tag comes from this function, never from the input.
func renderTranscriptMarkdown(src string, users, roles, channels map[string]string) string {
	var stash []string
	keep := func(s string) string {
		stash = append(stash, s)
		return fmt.Sprintf("\x00%d\x00", len(stash)-1)
	}

	s := strings.ReplaceAll(src, "\x00", "")
	s = reFence.ReplaceAllStringFunc(s, func(m string) string {
		code := reFence.FindStringSubmatch(m)[1]
		code = strings.Trim(code, "\n")
		return keep(fmt.Sprintf(`<pre class="md-pre"><code>%s</code></pre>`, html.EscapeString(code)))
	})
	s = reInline.ReplaceAllStringFunc(s, func(m string) string {
		code := reInline.FindStringSubmatch(m)[1]
		return keep(fmt.Sprintf(`<code class="md-code">%s</code>`, html.EscapeString(code)))
	})

	s = html.EscapeString(s)

	lookup := func(m map[string]string, id, fallback string) string {
		if name, ok := m[id]; ok {
			return name
		}
		return fallback
	}
	s = reUserMent.ReplaceAllStringFunc(s, func(m string) string {
		id := reUserMent.FindStringSubmatch(m)[1]
		return keep(fmt.Sprintf(`<span class="md-mention">@%s</span>`, html.EscapeString(lookup(users, id, "unknown-user"))))
	})
	s = reRoleMent.ReplaceAllStringFunc(s, func(m string) string {
		id := reRoleMent.FindStringSubmatch(m)[1]
		return keep(fmt.Sprintf(`<span class="md-mention">@%s</span>`, html.EscapeString(lookup(roles, id, "deleted-role"))))
	})
	s = reChanMent.ReplaceAllStringFunc(s, func(m string) string {
		id := reChanMent.FindStringSubmatch(m)[1]
		return keep(fmt.Sprintf(`<span class="md-mention">#%s</span>`, html.EscapeString(lookup(channels, id, "deleted-channel"))))
	})
	s = reEmoji.ReplaceAllString(s, `:$1:`)
	s = reTime.ReplaceAllStringFunc(s, func(m string) string {
		secs, _ := strconv.ParseInt(reTime.FindStringSubmatch(m)[1], 10, 64)
		return keep(fmt.Sprintf(`<span class="md-code">%s</span>`, html.EscapeString(time.Unix(secs, 0).Local().Format("Jan 2, 2006, 3:04 PM"))))
	})
	s = reMdLink.ReplaceAllStringFunc(s, func(m string) string {
		g := reMdLink.FindStringSubmatch(m)
		return keep(mdLink(g[2], g[1]))
	})
	s = reBareURL.ReplaceAllStringFunc(s, func(m string) string {
		g := reBareURL.FindStringSubmatch(m)
		return g[1] + keep(mdLink(g[2], g[2]))
	})
	s = reBold.ReplaceAllString(s, `<strong>$1</strong>`)
	s = reUnderln.ReplaceAllString(s, `<u>$1</u>`)
	s = reItalicS.ReplaceAllString(s, `<em>$1</em>`)
	s = reItalicU.ReplaceAllString(s, `<em>$1</em>`)
	s = reStrike.ReplaceAllString(s, `<s>$1</s>`)
	s = reSpoiler.ReplaceAllString(s, `<span class="md-spoiler">$1</span>`)

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if hm := reHeading.FindStringSubmatch(line); hm != nil {
			lines[i] = fmt.Sprintf(`<span class="md-h%d">%s</span>`, len(hm[1]), hm[2])
		} else if qm := reQuote.FindStringSubmatch(line); qm != nil {
			lines[i] = fmt.Sprintf(`<span class="md-quote">%s</span>`, qm[1])
		}
	}
	s = strings.Join(lines, "\n")

	for i := 0; i < 5 && strings.ContainsRune(s, 0); i++ {
		s = rePlaceholder.ReplaceAllStringFunc(s, func(m string) string {
			g := rePlaceholder.FindStringSubmatch(m)
			n, _ := strconv.Atoi(g[1])
			if n < len(stash) {
				return stash[n]
			}
			return ""
		})
	}
	return s
}

// --- template ---

var transcriptFuncs = template.FuncMap{
	"md": func(v transcriptView, s string) template.HTML {
		return template.HTML(renderTranscriptMarkdown(s, v.users, v.roles, v.channels))
	},
	"time": func(t any) string {
		switch v := t.(type) {
		case time.Time:
			return v.Local().Format("Jan 2, 2006, 3:04 PM")
		case *time.Time:
			if v == nil {
				return ""
			}
			return v.Local().Format("Jan 2, 2006, 3:04 PM")
		default:
			return ""
		}
	},
	"size": func(n int) string {
		if n > 1_048_576 {
			return fmt.Sprintf("%.1f MB", float64(n)/1_048_576)
		}
		kb := n / 1024
		if kb < 1 {
			kb = 1
		}
		return fmt.Sprintf("%d KB", kb)
	},
	"isImage": func(a store.Attachment) bool { return strings.HasPrefix(a.ContentType, "image/") },
	"hex":     func(n int) string { return fmt.Sprintf("#%06x", n) },
	"first":   func(ms []store.TicketMessage) store.TicketMessage { return ms[0] },
}

var transcriptTemplate = template.Must(template.New("transcript").Funcs(transcriptFuncs).Parse(transcriptHTML))

const transcriptHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Ticket #{{.Ticket.Number}} transcript{{if .Guild.Name}} &middot; {{.Guild.Name}}{{end}}</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
	:root { color-scheme: light dark; }
	body { margin: 0; padding: 24px 16px 48px; background: #f5f5f4; font: 15px/1.4 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #1c1917; }
	main { max-width: 760px; margin: 0 auto; }
	header.summary { border: 1px solid #d6d3d1; border-radius: 12px; padding: 16px 20px; margin-bottom: 16px; background: #fff; }
	header.summary h1 { font-size: 18px; margin: 0 0 4px; }
	header.summary .guild { color: #78716c; font-size: 13px; margin-bottom: 10px; }
	header.summary dl { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px 20px; margin: 10px 0 0; padding-top: 10px; border-top: 1px solid #e7e5e4; font-size: 13px; }
	header.summary dt { color: #78716c; font-size: 11px; }
	header.summary dd { margin: 2px 0 0; }
	section.conversation { overflow: hidden; border-radius: 12px; padding: 16px 0; font-size: 15px; line-height: 1.375; background: #313338; color: #dbdee1; }
	.note { margin: 16px 16px 0; border: 1px dashed #4e5058; border-radius: 6px; background: #2b2d31; padding: 8px 12px; }
	.note:first-child { margin-top: 0; }
	.note .who { display: flex; align-items: center; gap: 8px; font-size: 12px; color: #949ba4; }
	.note .who b { color: #dbdee1; font-weight: 500; }
	.note .body { margin-top: 4px; font-size: 14px; white-space: pre-wrap; overflow-wrap: break-word; }
	article { margin-top: 16px; display: flex; gap: 16px; padding: 2px 16px; }
	article:first-child { margin-top: 0; }
	article img.avatar { width: 40px; height: 40px; border-radius: 50%; margin-top: 2px; flex: none; }
	article .avatar-fallback { width: 40px; height: 40px; border-radius: 50%; margin-top: 2px; flex: none; background: #5865f2; }
	.msgs { min-width: 0; flex: 1; }
	.msgs .head { display: flex; align-items: baseline; gap: 8px; }
	.msgs .head .name { font-weight: 500; color: #fff; }
	.msgs .head .bot { border-radius: 4px; background: #5865f2; padding: 1px 5px; font-size: 10px; font-weight: 600; color: #fff; }
	.msgs .head time { font-size: 11px; color: #949ba4; }
	.msg { position: relative; margin-top: 4px; overflow-wrap: break-word; white-space: pre-wrap; }
	.msg.deleted { opacity: .6; text-decoration: line-through; }
	.msg .edited { font-size: 10px; color: #949ba4; text-decoration: none; }
	.deleted-note { font-size: 11px; color: #f0616d; }
	.embed { margin-top: 4px; max-width: 480px; border-left: 4px solid #1e1f22; border-radius: 4px; background: #2b2d31; padding: 10px 16px 10px 12px; }
	.embed .author { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; font-size: 14px; font-weight: 600; color: #fff; }
	.embed .author img { width: 24px; height: 24px; border-radius: 50%; }
	.embed .title { font-weight: 600; color: #fff; }
	.embed .desc { margin-top: 4px; font-size: 14px; white-space: pre-wrap; overflow-wrap: break-word; }
	.embed .field { margin-top: 8px; font-size: 14px; }
	.embed .field .name { font-weight: 600; color: #fff; }
	.embed .field .value { white-space: pre-wrap; overflow-wrap: break-word; }
	.embed .footer { margin-top: 8px; font-size: 12px; color: #949ba4; }
	.attachment-img { display: block; margin-top: 4px; width: fit-content; }
	.attachment-img img { max-height: 288px; max-width: 100%; border-radius: 6px; }
	.attachment-file { display: flex; align-items: center; gap: 12px; width: fit-content; max-width: 100%; margin-top: 4px; border: 1px solid #1e1f22; border-radius: 6px; background: #2b2d31; padding: 8px 12px; text-decoration: none; }
	.attachment-file .name { display: block; color: #00a8fc; font-size: 14px; }
	.attachment-file .size { display: block; font-size: 12px; color: #949ba4; }
	.md-code { border-radius: 4px; background: #1e1f22; padding: 1px 4px; font-family: ui-monospace, monospace; font-size: .85em; }
	.md-pre { display: block; margin: 4px 0; overflow-x: auto; border: 1px solid #1e1f22; border-radius: 6px; background: #2b2d31; padding: 8px; font-family: ui-monospace, monospace; font-size: .85em; }
	.md-mention { border-radius: 4px; background: rgba(88,101,242,.3); padding: 0 2px; font-weight: 500; color: #c9cdfb; }
	.md-link { color: #00a8fc; }
	.md-quote { display: block; border-left: 4px solid #4e5058; padding-left: 12px; }
	.md-spoiler { border-radius: 4px; background: #1e1f22; padding: 0 2px; }
	.md-h1 { display: block; font-size: 1.25rem; font-weight: 700; }
	.md-h2 { display: block; font-size: 1.125rem; font-weight: 700; }
	.md-h3 { display: block; font-weight: 700; }
	.empty { padding: 40px 20px; text-align: center; font-size: 14px; color: #949ba4; }
	.footnote { margin-top: 12px; text-align: center; font-size: 12px; color: #78716c; }
	a { color: inherit; }
</style>
</head>
<body>
<main>
	<header class="summary">
		{{if .Guild.Name}}<div class="guild">{{.Guild.Name}}</div>{{end}}
		<h1>Ticket #{{.Ticket.Number}}{{if .Ticket.TypeName}} &middot; {{.Ticket.TypeName}}{{end}}</h1>
		<dl>
			<div><dt>Opened by</dt><dd>{{.Ticket.OpenerName}}</dd></div>
			<div><dt>Opened</dt><dd>{{time .Ticket.OpenedAt}}</dd></div>
			<div><dt>Claimed by</dt><dd>{{if .Ticket.ClaimedByName}}{{.Ticket.ClaimedByName}}{{else}}Nobody{{end}}</dd></div>
			{{if eq (print .Ticket.Status) "closed"}}
			<div><dt>Closed</dt><dd>{{if .Ticket.ClosedAt}}{{time .Ticket.ClosedAt}}{{end}}{{if .Ticket.ClosedByName}} by {{.Ticket.ClosedByName}}{{end}}</dd></div>
			{{else}}
			<div><dt>Last message</dt><dd>{{time .Ticket.LastActivityAt}}</dd></div>
			{{end}}
			{{if .Ticket.CloseReason}}<div style="grid-column:1/-1"><dt>Close reason</dt><dd>{{.Ticket.CloseReason}}</dd></div>{{end}}
		</dl>
	</header>
	<section class="conversation">
		{{if not .Items}}<p class="empty">No messages were saved for this ticket.</p>{{end}}
		{{$v := .}}
		{{range .Items}}
			{{if .Note}}
				{{$note := .Note}}
				<aside class="note">
					<div class="who">🔒 Private note from <b>{{$note.AuthorName}}</b> &middot; <time>{{time $note.CreatedAt}}</time></div>
					<div class="body">{{md $v $note.Content}}</div>
				</aside>
			{{else}}
				{{$first := first .Messages}}
				<article>
					{{if $first.AuthorAvatar}}<img class="avatar" src="{{$first.AuthorAvatar}}" alt="">{{else}}<div class="avatar-fallback"></div>{{end}}
					<div class="msgs">
						<div class="head">
							<span class="name">{{$first.AuthorName}}</span>
							{{if $first.AuthorBot}}<span class="bot">APP</span>{{end}}
							<time>{{time $first.CreatedAt}}</time>
						</div>
						{{range .Messages}}
							{{$m := .}}
							<div class="msg{{if $m.DeletedAt}} deleted{{end}}">
								{{if $m.Content}}{{md $v $m.Content}}{{if $m.EditedAt}} <span class="edited">(edited)</span>{{end}}{{end}}
							</div>
							{{if $m.DeletedAt}}<div class="deleted-note">Deleted message</div>{{end}}
							{{range $m.Embeds}}
								{{$e := .}}
								<div class="embed" style="border-color:{{if $e.Color}}{{hex $e.Color}}{{else}}#1e1f22{{end}}">
									{{if $e.Author}}<div class="author">{{if $e.Author.IconURL}}<img src="{{$e.Author.IconURL}}" alt="">{{end}}{{$e.Author.Name}}</div>{{end}}
									{{if $e.Title}}<div class="title">{{$e.Title}}</div>{{end}}
									{{if $e.Description}}<div class="desc">{{md $v $e.Description}}</div>{{end}}
									{{range $e.Fields}}<div class="field"><div class="name">{{.Name}}</div><div class="value">{{md $v .Value}}</div></div>{{end}}
									{{if $e.Footer}}<div class="footer">{{$e.Footer}}</div>{{end}}
								</div>
							{{end}}
							{{range $m.Attachments}}
								{{if isImage .}}
									<a class="attachment-img" href="{{.URL}}" target="_blank" rel="noopener noreferrer"><img src="{{.URL}}" alt="{{.Name}}"></a>
								{{else}}
									<a class="attachment-file" href="{{.URL}}" target="_blank" rel="noopener noreferrer">
										<span><span class="name">{{.Name}}</span><span class="size">{{size .Size}}</span></span>
									</a>
								{{end}}
							{{end}}
						{{end}}
					</div>
				</article>
			{{end}}
		{{end}}
	</section>
	<p class="footnote">Downloaded transcript &middot; attachment and avatar links are hosted by Discord and may stop working after a while.</p>
</main>
</body>
</html>
`
