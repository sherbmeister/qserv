package main

import "strings"

type Handler func(*Link, *Message)

type Bus struct {
	handlers map[string][]Handler
	log      *Logger
}

func NewBus(log *Logger) *Bus {
	return &Bus{handlers: make(map[string][]Handler), log: log}
}

func (b *Bus) On(verb string, h Handler) {
	verb = strings.ToUpper(verb)
	b.handlers[verb] = append(b.handlers[verb], h)
}

func (b *Bus) Emit(l *Link, msg *Message) {
	verb := strings.ToUpper(msg.Command)
	if hs, ok := b.handlers[verb]; ok {
		for _, h := range hs {
			h(l, msg)
		}
	}
	if hs, ok := b.handlers["*"]; ok {
		for _, h := range hs {
			h(l, msg)
		}
	}
}
