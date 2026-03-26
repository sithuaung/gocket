package auth

import (
	"fmt"
	"strings"
)

func ValidatePrivateAuth(appKey, appSecret, socketID, channelName, authToken string) bool {
	parts := strings.SplitN(authToken, ":", 2)
	if len(parts) != 2 || parts[0] != appKey {
		return false
	}

	message := fmt.Sprintf("%s:%s", socketID, channelName)
	expected := HmacSHA256Hex(appSecret, message)
	return HmacEqual(expected, parts[1])
}

func ValidatePresenceAuth(appKey, appSecret, socketID, channelName, channelData, authToken string) bool {
	parts := strings.SplitN(authToken, ":", 2)
	if len(parts) != 2 || parts[0] != appKey {
		return false
	}

	message := fmt.Sprintf("%s:%s:%s", socketID, channelName, channelData)
	expected := HmacSHA256Hex(appSecret, message)
	return HmacEqual(expected, parts[1])
}
