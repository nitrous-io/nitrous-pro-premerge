package pubsub_test

import (
	"fmt"
	"testing"

	"github.com/nitrous-io/nitrous/pkg/pubsub"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pubsub")
}

var _ = Describe("pubsub", func() {
	Describe("Subscribe", func() {
		It("returns a channel that receives published messages", func(done Done) {
			ps := pubsub.New(1)
			ch := ps.Subscribe("topic-1")
			ps.Publish("message 1", "topic-1")

			Expect(<-ch).To(Equal("message 1"))
			close(done)
		})

		It("works with multiple topics", func(done Done) {
			ps := pubsub.New(3)
			ch := ps.Subscribe("topic-1", "topic-2")
			ps.Publish("message 1", "topic-1")
			ps.Publish("message 2", "topic-2")
			ps.Publish("message 3", "topic-1")

			Expect(<-ch).To(Equal("message 1"))
			Expect(<-ch).To(Equal("message 2"))
			Expect(<-ch).To(Equal("message 3"))
			close(done)
		})
	})

	Describe("Unsubscribe", func() {
		It("channel no longer receives messages published to the topic", func(done Done) {
			ps := pubsub.New(1)
			ch := ps.Subscribe("topic-1")

			ps.Unsubscribe(ch, "topic-1")
			ps.Publish("message 1", "topic-1")

			select {
			case v, ok := <-ch:
				if !ok {
					Fail("Channel unexpectedly closed")
				}
				Fail(fmt.Sprintf("Unexpected receive on unsubscribed channel: %v", v))
			default:
				// For non-blocking receive.
			}

			close(done)
		})

		It("unsubscribes from only the specified topics", func(done Done) {
			ps := pubsub.New(1)
			ch := ps.Subscribe("topic-1", "topic-2")

			ps.Unsubscribe(ch, "topic-1")
			ps.Publish("m1", "topic-1")
			ps.Publish("m2", "topic-2")

			Expect(<-ch).To(Equal("m2"))

			close(done)
		})

		It("unsubscribes from all topics if no topic is specified", func(done Done) {
			ps := pubsub.New(1)
			ch := ps.Subscribe("topic-1", "topic-2")

			ps.Unsubscribe(ch)
			ps.Publish("message 1", "topic-1")
			ps.Publish("message 2", "topic-2")

			select {
			case v, ok := <-ch:
				if !ok {
					Fail("Channel unexpectedly closed")
				}
				Fail(fmt.Sprintf("Unexpected receive on unsubscribed channel: %v", v))
			default:
				// For non-blocking receive.
			}

			close(done)
		})

		It("does not close the channel", func() {
			ps := pubsub.New(1)
			ch := ps.Subscribe("topic-1")
			ps.Unsubscribe(ch, "topic-1")

			Expect(ch).NotTo(BeClosed())
		})
	})

	Describe("Close", func() {
		It("closes a topic", func(done Done) {
			ps := pubsub.New(2)
			ch := ps.Subscribe("topic-1")

			ps.Publish("m1", "topic-1")
			Expect(<-ch).To(Equal("m1"))

			ps.Close("topic-1")

			ps.Publish("m2", "topic-1")
			select {
			case v, ok := <-ch:
				if !ok {
					Fail("Channel unexpectedly closed")
				}
				Fail(fmt.Sprintf("Unexpected receive on closed topic: %v", v))
			default:
				// For non-blocking receive.
			}

			close(done)
		})
	})

	Describe("Publish", func() {
		It("publishes messages to topic subscribers", func(done Done) {
			ps := pubsub.New(1)
			ch1 := ps.Subscribe("topic-1")
			ch2 := ps.Subscribe("topic-1")
			ch3 := ps.Subscribe("topic-2")

			ps.Publish("m1", "topic-1")
			ps.Publish("m2", "topic-2")

			Expect(<-ch1).To(Equal("m1"))
			Expect(<-ch2).To(Equal("m1"))
			Expect(<-ch3).To(Equal("m2"))

			close(done)
		})

		It("supports publishing messages to multiple topics", func(done Done) {
			ps := pubsub.New(1)
			ch1 := ps.Subscribe("topic-1")
			ch2 := ps.Subscribe("topic-1")
			ch3 := ps.Subscribe("topic-2")

			ps.Publish("m1", "topic-1", "topic-2")
			Expect(<-ch1).To(Equal("m1"))
			Expect(<-ch2).To(Equal("m1"))
			Expect(<-ch3).To(Equal("m1"))

			close(done)
		})

		It("allows publishing to a topic with no subscribers", func(done Done) {
			ps := pubsub.New(1)
			ps.Publish("nobody listens to me", "topic-1")
			close(done)
		}, 0.1)
	})

	Describe("Broadcast", func() {
		It("publishes a message to all subscribers", func(done Done) {
			ps := pubsub.New(1)
			ch1 := ps.Subscribe("topic-1")
			ch2 := ps.Subscribe("topic-2")

			ps.Broadcast("m1")

			Expect(<-ch1).To(Equal("m1"))
			Expect(<-ch2).To(Equal("m1"))

			close(done)
		})

		It("publishes message to each subscriber only once", func(done Done) {
			ps := pubsub.New(1)

			// Subscribe to multiple topics.
			ch := ps.Subscribe("topic-1", "topic-2")

			ps.Broadcast("m1")

			Expect(<-ch).To(Equal("m1"))
			select {
			case v, ok := <-ch:
				if !ok {
					Fail("Channel unexpectedly closed")
				}
				Fail(fmt.Sprintf("Unexpected receive: %v", v))
			default:
				// For non-blocking receive.
			}

			close(done)
		})
	})

	Describe("Clear", func() {
		It("closes all subscriber channels", func(done Done) {
			ps := pubsub.New(1)
			ch1 := ps.Subscribe("topic-1")
			ch2 := ps.Subscribe("topic-2")

			ps.Clear()

			Expect(ch1).To(BeClosed())
			Expect(ch2).To(BeClosed())

			close(done)
		})

		It("resets", func(done Done) {
			ps := pubsub.New(1)
			ch1 := ps.Subscribe("topic-1")

			ps.Publish("m1", "topic-1")
			Expect(<-ch1).To(Equal("m1"))

			ps.Clear()

			Expect(ch1).To(BeClosed())

			ps.Publish("m2", "topic-1") // This message is "lost".

			ch2 := ps.Subscribe("topic-1")
			ps.Publish("m3", "topic-1")

			Expect(<-ch2).To(Equal("m3"))

			close(done)
		})
	})
})
