package main

import "sync"

type EventBus struct {
	mu   sync.RWMutex
	subs map[string][]func(any)
}

func NewEventBus() *EventBus {
	return &EventBus{
		subs: make(map[string][]func(any)),
	}
}

func (b *EventBus) Subscribe(topic string, fn func(any)) {
	b.mu.Lock()
	b.subs[topic] = append(b.subs[topic], fn)
	b.mu.Unlock()
}

func (b *EventBus) Publish(topic string, payload any) {
	b.mu.RLock()
	fns := b.subs[topic]
	b.mu.RUnlock()
	for _, fn := range fns {
		fn(payload)
	}
}
