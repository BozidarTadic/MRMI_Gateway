package inbox

import "sync"

// Event is emitted for each envelope that passed policy evaluation with ALLOW.
type Event struct {
	IdempotencyKey  string `json:"idempotency_key"`
	SenderRegion    string `json:"sender_region"`
	RecipientRegion string `json:"recipient_region"`
	TrustTier       uint32 `json:"trust_tier"`
	Payload         []byte `json:"payload,omitempty"`
	Timestamp       int64  `json:"timestamp"`
}

// Inbox is a fan-out broadcaster. Subscribers each receive a buffered channel
// of Events. A slow subscriber is dropped (channel full = event skipped) to
// avoid head-of-line blocking for other subscribers.
type Inbox struct {
	mu   sync.Mutex
	subs map[uint64]chan Event
	next uint64
}

const subBufSize = 64

func New() *Inbox {
	return &Inbox{subs: make(map[uint64]chan Event)}
}

// Subscribe registers a new subscriber and returns its ID and event channel.
func (in *Inbox) Subscribe() (id uint64, ch <-chan Event) {
	in.mu.Lock()
	defer in.mu.Unlock()
	id = in.next
	in.next++
	c := make(chan Event, subBufSize)
	in.subs[id] = c
	return id, c
}

// Unsubscribe removes the subscriber and closes its channel.
func (in *Inbox) Unsubscribe(id uint64) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if c, ok := in.subs[id]; ok {
		close(c)
		delete(in.subs, id)
	}
}

// Publish sends an event to all current subscribers. Non-blocking per subscriber.
func (in *Inbox) Publish(e Event) {
	in.mu.Lock()
	defer in.mu.Unlock()
	for _, c := range in.subs {
		select {
		case c <- e:
		default:
		}
	}
}
