// nickmap.go
package main

import "strings"

// Simple in-memory maps for UID <-> nick.
var (
	uidToNick = make(map[string]string)
	nickToUID = make(map[string]string) // lowercase nick -> UID
)

// setNick stores/updates the mapping of a UID to a nick.
func setNick(uid, nick string) {
	if old, ok := uidToNick[uid]; ok {
		delete(nickToUID, strings.ToLower(old))
	}
	uidToNick[uid] = nick
	nickToUID[strings.ToLower(nick)] = uid
}

// getNick returns the current nick for a UID (empty if unknown).
func getNick(uid string) string {
	return uidToNick[uid]
}

// delNick removes a UID and its reverse nick mapping.
func delNick(uid string) {
	if n, ok := uidToNick[uid]; ok {
		delete(nickToUID, strings.ToLower(n))
		delete(uidToNick, uid)
	}
}

// (Optional utility if you ever need it)
// getUIDByNick returns the UID for a given nick (case-insensitive).
func getUIDByNick(nick string) string {
	return nickToUID[strings.ToLower(nick)]
}
