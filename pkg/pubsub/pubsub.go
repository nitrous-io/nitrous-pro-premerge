package pubsub

import "sync"

type PubSub struct {
	sync.RWMutex
	registry map[Topic]map[chan Message]struct{}
	capacity int
}

type Topic string

type Message interface{}

// New creates a new PubSub. The given capacity determines the capacity of
// channels created and returned.
func New(capacity int) *PubSub {
	return &PubSub{
		registry: make(map[Topic]map[chan Message]struct{}),
		capacity: capacity,
	}
}

// Subscribe returns a buffered channel that receives Messages about 1 or more
// Topics.
// FIXME Should return `<-chan Message` but it's hard because Unsubscribe() takes
// a channel and `<-chan Message` is not the same type as `chan Message`. Doing
// this will require an additional tracking data structure.
func (ps *PubSub) Subscribe(topics ...Topic) chan Message {
	ch := make(chan Message, ps.capacity)
	ps.Lock()
	for _, topic := range topics {
		ps.initTopic(topic)
		ps.registry[topic][ch] = struct{}{}
	}
	ps.Unlock()
	return ch
}

// Unsubscribe unsubscribes a channel from 1 or more Topics. If no topic is
// specified, it is unsubscribed from all topics.
// If the channel is no longer subscribed to any Topic, it will be closed.
func (ps *PubSub) Unsubscribe(ch chan Message, topics ...Topic) {
	ps.Lock()
	defer ps.Unlock()

	// No topic specified - unsubscribe from all topics.
	if len(topics) == 0 {
		topics = make([]Topic, len(ps.registry))
		for t := range ps.registry {
			topics = append(topics, t)
		}
	}

	for _, topic := range topics {
		delete(ps.registry[topic], ch)
	}

	for _, chMap := range ps.registry {
		if _, ok := chMap[ch]; ok {
			return
		}
	}

	// Channel is no longer subscribed to any Topic so we close it.
	close(ch)
}

// Close removes all subscribers from 1 or more Topics.
func (ps *PubSub) Close(topics ...Topic) {
	for _, topic := range topics {
		delete(ps.registry, topic)
	}
}

// Publish publishes a message to subscribers of 1 or more Topics.
// If a subscriber is subscribed to > 1 of the given topics, it will receive
// a message for each topic (i.e. duplicate messages).
func (ps *PubSub) Publish(msg Message, topics ...Topic) {
	ps.RLock()
	for _, topic := range topics {
		// FIXME We might want to uniq the channels if we don't want to send
		// duplicate messages.
		for ch, _ := range ps.registry[topic] {
			ch <- msg
		}
	}
	ps.RUnlock()
}

// Broadcast publishes a message to all subscribers. Each subscriber will only
// receive the message once.
func (ps *PubSub) Broadcast(msg Message) {
	ps.RLock()
	chans := ps.allSubscribers()
	ps.RUnlock()

	for ch, _ := range chans {
		ch <- msg
	}
}

// Clear closes all subscriber channels and resets topics and subscribers.
// Useful for cleaning up goroutines in callers.
func (ps *PubSub) Clear() {
	ps.Lock()
	chans := ps.allSubscribers()
	for ch, _ := range chans {
		close(ch)
	}

	ps.registry = make(map[Topic]map[chan Message]struct{})
	ps.Unlock()
}

func (ps *PubSub) initTopic(topic Topic) {
	if ps.registry[topic] == nil {
		ps.registry[topic] = make(map[chan Message]struct{})
	}
}

func (ps *PubSub) allSubscribers() map[chan Message]struct{} {
	chans := make(map[chan Message]struct{})
	for _, chMap := range ps.registry {
		for k, v := range chMap {
			chans[k] = v
		}
	}
	return chans
}
