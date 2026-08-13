package queue

import "sync"

// EventKind discriminates queue events.
type EventKind string

const (
	EventProgress EventKind = "progress" // stage/progress changed
	EventStatus   EventKind = "status"   // job entered/left a status
	EventLog      EventKind = "log"      // one new log line
)

// Event is a typed queue notification. The web layer renders these into
// HTML fragments for htmx's SSE extension; the broker itself knows nothing
// about templates.
type Event struct {
	Kind     EventKind
	JobID    string
	Stage    string
	Progress float64
	Status   string
	Line     string
}

// Broker is a simple fan-out pub/sub. Slow subscribers drop events rather
// than block the workers — progress is idempotent, the next event corrects.
type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: map[chan Event]struct{}{}}
}

func (b *Broker) Subscribe() (ch chan Event, cancel func()) {
	ch = make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

func (b *Broker) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // subscriber is behind; drop
		}
	}
}
