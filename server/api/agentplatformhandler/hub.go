package agentplatformhandler

import (
	"sync"
)

// -----------------------------------------------------------------------------
// In-process event hub (M2)
//
// The hub fan-outs realtime events (task progress, issue/comment mutations)
// from writers (workers, HTTP handlers) to subscribers (WebSocket connections).
// Keyed by workspace id so a client only sees events for the workspace it
// asked for.
//
// This is deliberately in-process only. Design doc §4.3: "we can swap to
// Redis later if we scale beyond one server process." When that happens,
// Publish stays the same shape; Subscribe fan-out wires to a Redis channel.
// -----------------------------------------------------------------------------

// WSEvent is the payload shape broadcast to subscribers. Its Type/Payload
// are marshaled as the JSON sent over the WebSocket.
type WSEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type subscriber struct {
	ch     chan WSEvent
	closed bool
	mu     sync.Mutex
}

// Hub is the pubsub router. Use InitHub / globalHub; tests may construct
// their own via newHub.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*subscriber]struct{}
}

var globalHub *Hub

// InitHub creates the process-wide hub. Safe to call more than once; only
// the first call creates. Returns the hub for chain convenience.
func InitHub() *Hub {
	if globalHub == nil {
		globalHub = newHub()
	}
	return globalHub
}

func newHub() *Hub {
	return &Hub{subs: make(map[string]map[*subscriber]struct{})}
}

// Subscribe registers a new subscriber for the given workspace. Returns:
//   - a receive-only channel that emits events until the subscriber is
//     unsubscribed or overflows
//   - an unsubscribe func the caller MUST invoke on cleanup (e.g. via defer)
//
// The channel is buffered (32 slots). If the caller drains slower than
// Publish produces, the subscriber is dropped — the client should
// reconnect and reload state.
func (h *Hub) Subscribe(workspaceID string) (<-chan WSEvent, func()) {
	s := &subscriber{ch: make(chan WSEvent, 32)}
	h.mu.Lock()
	set, ok := h.subs[workspaceID]
	if !ok {
		set = make(map[*subscriber]struct{})
		h.subs[workspaceID] = set
	}
	set[s] = struct{}{}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		if set, ok := h.subs[workspaceID]; ok {
			delete(set, s)
			if len(set) == 0 {
				delete(h.subs, workspaceID)
			}
		}
		h.mu.Unlock()
		s.close()
	}
	return s.ch, unsub
}

// Publish broadcasts an event to every current subscriber of workspaceID.
// Non-blocking: if a subscriber's channel is full it is kicked (channel
// closed), so stream delivery never stalls a publisher.
//
// Safe to call when the hub has no subscribers — it no-ops.
func (h *Hub) Publish(workspaceID string, evt WSEvent) {
	if h == nil {
		return
	}
	h.mu.RLock()
	set := h.subs[workspaceID]
	if len(set) == 0 {
		h.mu.RUnlock()
		return
	}
	// Snapshot under RLock so we don't hold it during sends.
	snap := make([]*subscriber, 0, len(set))
	for s := range set {
		snap = append(snap, s)
	}
	h.mu.RUnlock()

	for _, s := range snap {
		if !s.trySend(evt) {
			// Overflow — kick the subscriber. The caller's unsub will
			// idempotently remove it from the set.
			s.close()
		}
	}
}

// publishTo is a convenience wrapper around globalHub.Publish that tolerates
// nil (useful in tests that don't init the hub and in code paths that
// fire before StartWorkers ran).
func publishTo(workspaceID, eventType string, payload any) {
	if globalHub == nil {
		return
	}
	globalHub.Publish(workspaceID, WSEvent{Type: eventType, Payload: payload})
}

// -----------------------------------------------------------------------------
// subscriber helpers
// -----------------------------------------------------------------------------

func (s *subscriber) trySend(evt WSEvent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- evt:
		return true
	default:
		return false
	}
}

func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}
