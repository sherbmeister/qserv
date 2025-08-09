package main

import "strings"

type Message struct {
	Tags     string
	Prefix   string
	Command  string
	Params   []string
	Trailing string
	Raw      string
}

func ParseLine(s string) *Message {
	if s == "" {
		return &Message{Raw: s}
	}
	msg := &Message{Raw: s}

	// Strip IRCv3 tags if present: "@tag;tag :rest"
	if s[0] == '@' {
		if sp := strings.IndexByte(s, ' '); sp >= 0 {
			msg.Tags = s[1:sp]
			s = s[sp+1:]
		} else {
			return msg
		}
	}

	// Prefix
	if len(s) > 0 && s[0] == ':' {
		if sp := strings.IndexByte(s, ' '); sp >= 0 {
			msg.Prefix = s[1:sp]
			s = s[sp+1:]
		} else {
			return msg
		}
	}

	// Trailing
	if tr := strings.Index(s, " :"); tr >= 0 {
		msg.Trailing = s[tr+2:]
		s = s[:tr]
	}

	parts := strings.Fields(s)
	if len(parts) == 0 {
		return msg
	}
	msg.Command = parts[0]
	if len(parts) > 1 {
		msg.Params = parts[1:]
	}
	return msg
}
