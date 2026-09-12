// Package discordx holds small helpers shared by the bot and API for working
// with the Discord API.
package discordx

import (
	"bytes"
	"errors"

	"github.com/disgoorg/disgo/rest"
)

// Discord JSON error codes we handle explicitly.
// https://discord.com/developers/docs/topics/opcodes-and-status-codes#json
const (
	CodeUnknownChannel     = 10003
	CodeUnknownMember      = 10007
	CodeUnknownMessage     = 10008
	CodeMaxChannels        = 30013
	CodeInvalidFormBody    = 50035
	CodeMissingAccess      = 50001
	CodeCannotDMUser       = 50007
	CodeMissingPermissions = 50013
)

// Code returns the Discord JSON error code of err, or 0.
func Code(err error) int {
	var re *rest.Error
	if errors.As(err, &re) {
		return int(re.Code)
	}
	return 0
}

func IsCode(err error, codes ...int) bool {
	c := Code(err)
	for _, code := range codes {
		if c == code {
			return true
		}
	}
	return false
}

// MaxChannelsPerCategory is Discord's limit on channels in one category.
const MaxChannelsPerCategory = 50

// IsCategoryFull reports whether Discord refused to create a channel because
// its category already holds MaxChannelsPerCategory channels. Discord reports
// this as an invalid form body whose parent_id error is CHANNEL_PARENT_MAX_CHANNELS.
func IsCategoryFull(err error) bool {
	var re *rest.Error
	return errors.As(err, &re) && int(re.Code) == CodeInvalidFormBody &&
		bytes.Contains(re.Errors, []byte("CHANNEL_PARENT_MAX_CHANNELS"))
}

// IsInvalidEmoji reports whether Discord rejected a message because a button
// or select option uses an emoji the bot can't use (one from a server it
// isn't in, or a deleted one).
func IsInvalidEmoji(err error) bool {
	var re *rest.Error
	return errors.As(err, &re) && int(re.Code) == CodeInvalidFormBody && bytes.Contains(re.Errors, []byte("INVALID_EMOJI"))
}

// Friendly returns a user-facing explanation for common Discord errors, or
// "" if the error isn't one we recognise.
func Friendly(err error) string {
	if IsInvalidEmoji(err) {
		return "Discord rejected one of the emoji: the bot can only use emoji from this server or standard ones. Check the emoji on each ticket type and try again."
	}
	if IsCategoryFull(err) {
		return "The category these tickets open in is full: Discord allows 50 channels per category. " +
			"The team needs to close some tickets, pick another category, or switch this ticket type to private threads."
	}
	switch Code(err) {
	case CodeMissingPermissions:
		return "I don't have permission to do that. Make sure my role has Manage Channels and Manage Roles, and can see the channel or category being used."
	case CodeMissingAccess:
		return "I can't access that channel. Make sure my role can view it."
	case CodeUnknownChannel:
		return "That channel no longer exists."
	case CodeMaxChannels:
		return "This server has hit Discord's 500 channel limit. Try switching the ticket type to private threads."
	}
	return ""
}
