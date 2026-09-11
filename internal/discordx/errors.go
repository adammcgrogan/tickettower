// Package discordx holds small helpers shared by the bot and API for working
// with the Discord API.
package discordx

import (
	"errors"

	"github.com/disgoorg/disgo/rest"
)

// Discord JSON error codes we handle explicitly.
// https://discord.com/developers/docs/topics/opcodes-and-status-codes#json
const (
	CodeUnknownChannel     = 10003
	CodeUnknownMessage     = 10008
	CodeMaxChannels        = 30013
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

// Friendly returns a user-facing explanation for common Discord errors, or
// "" if the error isn't one we recognise.
func Friendly(err error) string {
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
