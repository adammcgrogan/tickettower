package discordx

import (
	"testing"
	"time"
)

func TestAttachmentExpiry(t *testing.T) {
	signed := "https://cdn.discordapp.com/attachments/1/2/shot.png?ex=66f2c3a0&is=66f17220&hm=abc"
	want := time.Unix(0x66f2c3a0, 0)
	if got := AttachmentExpiry(signed); !got.Equal(want) {
		t.Errorf("expiry = %v, want %v", got, want)
	}
	for _, raw := range []string{"https://cdn.discordapp.com/attachments/1/2/shot.png", "https://example.com/?ex=zz", "::bad"} {
		if got := AttachmentExpiry(raw); !got.IsZero() {
			t.Errorf("expiry of %q = %v, want zero", raw, got)
		}
	}
}
