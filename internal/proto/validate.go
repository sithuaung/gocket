package proto

import (
	"fmt"
	"regexp"
)

const (
	MaxChannelNameLength = 200
	MaxEventNameLength   = 200
)

// channelNameRe matches the allowed characters in a Pusher channel name.
var channelNameRe = regexp.MustCompile(`^[A-Za-z0-9_\-=@,.;]+$`)

// ValidateChannelName checks that a channel name conforms to Pusher protocol rules.
func ValidateChannelName(name string) error {
	if name == "" {
		return fmt.Errorf("channel name must not be empty")
	}
	if len(name) > MaxChannelNameLength {
		return fmt.Errorf("channel name exceeds %d characters", MaxChannelNameLength)
	}
	if !channelNameRe.MatchString(name) {
		return fmt.Errorf("channel name contains invalid characters")
	}
	return nil
}

// ValidateEventName checks that an event name conforms to Pusher protocol rules.
func ValidateEventName(name string) error {
	if name == "" {
		return fmt.Errorf("event name must not be empty")
	}
	if len(name) > MaxEventNameLength {
		return fmt.Errorf("event name exceeds %d characters", MaxEventNameLength)
	}
	return nil
}
