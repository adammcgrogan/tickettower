package discordx

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/disgoorg/disgo/rest"
)

func TestIsCategoryFull(t *testing.T) {
	full := &rest.Error{Code: CodeInvalidFormBody, Message: "Invalid Form Body",
		Errors: json.RawMessage(`{"parent_id":{"_errors":[{"code":"CHANNEL_PARENT_MAX_CHANNELS","message":"Maximum number of channels in category reached (50)"}]}}`)}
	badName := &rest.Error{Code: CodeInvalidFormBody, Message: "Invalid Form Body",
		Errors: json.RawMessage(`{"name":{"_errors":[{"code":"BASE_TYPE_BAD_LENGTH","message":"Must be between 1 and 100 in length."}]}}`)}

	if !IsCategoryFull(fmt.Errorf("create channel: %w", full)) {
		t.Error("wrapped full-category error not recognised")
	}
	if IsCategoryFull(badName) || IsCategoryFull(&rest.Error{Code: CodeMaxChannels}) || IsCategoryFull(errors.New("x")) {
		t.Error("other errors treated as a full category")
	}
	if msg := Friendly(full); msg == "" {
		t.Error("no friendly message for a full category")
	}
	if msg := Friendly(badName); msg != "" {
		t.Errorf("unexpected friendly message for a bad name: %q", msg)
	}
}

func TestIsInvalidEmoji(t *testing.T) {
	bad := &rest.Error{Code: CodeInvalidFormBody, Message: "Invalid Form Body",
		Errors: json.RawMessage(`{"components":{"0":{"components":{"1":{"emoji":{"id":{"_errors":[{"code":"BUTTON_COMPONENT_INVALID_EMOJI","message":"Invalid emoji"}]}}}}}}}`)}
	if !IsInvalidEmoji(fmt.Errorf("publish: %w", bad)) || Friendly(bad) == "" {
		t.Error("invalid emoji error not recognised")
	}
	if IsInvalidEmoji(&rest.Error{Code: CodeInvalidFormBody}) || IsInvalidEmoji(errors.New("x")) {
		t.Error("other errors treated as an invalid emoji")
	}
}
