package draupnir

import "sync"

// Broker is a lightweight topic-based publish/subscribe bus used to fan events
// out to Server-Sent Events clients (see Router.EVENTSTREAM). It is safe for
// concurrent use.
//
// Publishing never blocks: if a subscriber's buffer is full (a slow consumer),
// the event is dropped for that subscriber rather than stalling the bus.
type Broker struct {
	mu     sync.RWMutex
	topics map[string]map[*Subscription]struct{}
	buffer int
	closed bool
}

// Subscription is a single client's stream of events for a topic.
type Subscription struct {
	broker *Broker
	topic  string
	C      chan Event
	once   sync.Once
}

// BrokerOption configures a Broker.
type BrokerOption func(*Broker)

// WithSubscriberBuffer sets the per-subscriber channel buffer size (default 16).
func WithSubscriberBuffer(n int) BrokerOption {
	return func(b *Broker) {
		if n > 0 {
			b.buffer = n
		}
	}
}

// NewBroker creates an empty broker.
func NewBroker(opts ...BrokerOption) *Broker {
	b := &Broker{
		topics: make(map[string]map[*Subscription]struct{}),
		buffer: 16,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Subscribe registers a new subscription for topic.
func (b *Broker) Subscribe(topic string) *Subscription {
	sub := &Subscription{
		broker: b,
		topic:  topic,
		C:      make(chan Event, b.buffer),
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(sub.C)
		return sub
	}
	subs := b.topics[topic]
	if subs == nil {
		subs = make(map[*Subscription]struct{})
		b.topics[topic] = subs
	}
	subs[sub] = struct{}{}
	return sub
}

// Unsubscribe removes a subscription and closes its channel. It is idempotent.
func (b *Broker) Unsubscribe(sub *Subscription) {
	if sub == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if subs := b.topics[sub.topic]; subs != nil {
		if _, ok := subs[sub]; ok {
			delete(subs, sub)
			sub.once.Do(func() { close(sub.C) })
		}
		if len(subs) == 0 {
			delete(b.topics, sub.topic)
		}
	}
}

// Publish delivers ev to every subscriber of topic. Slow subscribers whose
// buffer is full are skipped (the event is dropped for them).
func (b *Broker) Publish(topic string, ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for sub := range b.topics[topic] {
		select {
		case sub.C <- ev:
		default: // drop for slow consumer
		}
	}
}

// Topics returns the list of topics with at least one subscriber.
func (b *Broker) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	topics := make([]string, 0, len(b.topics))
	for t := range b.topics {
		topics = append(topics, t)
	}
	return topics
}

// Close shuts the broker down, closing every subscriber channel. After Close,
// Subscribe returns an already-closed subscription and Publish is a no-op.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for topic, subs := range b.topics {
		for sub := range subs {
			sub.once.Do(func() { close(sub.C) })
		}
		delete(b.topics, topic)
	}
}
