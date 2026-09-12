package discordx

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/rest"
)

// AttachmentExpiry returns when a Discord CDN attachment link stops working.
// Attachment URLs are signed with an ex= query parameter (hex Unix seconds).
// It returns the zero time for links that aren't signed.
func AttachmentExpiry(raw string) time.Time {
	u, err := url.Parse(raw)
	if err != nil {
		return time.Time{}
	}
	ex, err := strconv.ParseInt(u.Query().Get("ex"), 16, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(ex, 0)
}

var refreshURLsEndpoint = rest.NewEndpoint(http.MethodPost, "/attachments/refresh-urls")

// RefreshAttachmentURLs asks Discord for freshly signed links for expired or
// expiring attachment URLs. The result maps each original URL to its new
// one; URLs Discord wouldn't refresh are left out.
func RefreshAttachmentURLs(client rest.Client, urls []string, opts ...rest.RequestOpt) (map[string]string, error) {
	var rs struct {
		Refreshed []struct {
			Original  string `json:"original"`
			Refreshed string `json:"refreshed"`
		} `json:"refreshed_urls"`
	}
	rq := map[string][]string{"attachment_urls": urls}
	if err := client.Do(refreshURLsEndpoint.Compile(nil), rq, &rs, opts...); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rs.Refreshed))
	for _, r := range rs.Refreshed {
		if r.Refreshed != "" {
			out[r.Original] = r.Refreshed
		}
	}
	return out, nil
}
