package server

import (
	"sync"

	"timenotesblog/internal/protocol"
)

type eventAudience int

const (
	audiencePublic eventAudience = iota
	audienceAdmin
	audienceAll
)

type eventHub struct {
	mu      sync.RWMutex
	clients map[*clientSession]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[*clientSession]struct{})}
}

func (e *eventHub) add(cs *clientSession) {
	e.mu.Lock()
	e.clients[cs] = struct{}{}
	e.mu.Unlock()
}

func (e *eventHub) remove(cs *clientSession) {
	e.mu.Lock()
	delete(e.clients, cs)
	e.mu.Unlock()
}

func (e *eventHub) broadcast(msgType string, payload any, audience eventAudience) {
	env, err := protocol.NewEnvelope(msgType, "", payload)
	if err != nil {
		return
	}
	e.mu.RLock()
	targets := make([]*clientSession, 0, len(e.clients))
	for cs := range e.clients {
		if e.eligible(cs, audience) {
			targets = append(targets, cs)
		}
	}
	e.mu.RUnlock()
	for _, cs := range targets {
		cs.reply(env)
	}
}

func (e *eventHub) eligible(cs *clientSession, audience eventAudience) bool {
	if cs == nil {
		return false
	}
	switch audience {
	case audiencePublic:
		return true
	case audienceAdmin:
		return cs.user != nil && cs.user.Role == "admin"
	case audienceAll:
		return true
	default:
		return false
	}
}
