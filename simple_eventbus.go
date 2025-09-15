package eventbus

import (
	"context"
	"runtime"
	"sync"
)

type subscription[T any] struct {
	events  chan T
	wg      sync.WaitGroup
	lock    sync.RWMutex
	stopped bool
}

// SimpleEventBus is the simplest implementation of an event bus that allows for subscription cancellation.
//
// Other event buses (supporting event filtering and handler registration for example) can be built on top.
type SimpleEventBus[T any] struct {
	events        chan T
	subscriptions map[*subscription[T]]struct{}
	lock          sync.RWMutex
}

// NewSimpleEventBus instantiates a new SimpleEventBus for the given event type.
//
// A runtime cleanup function is registered to close the events channel and thereby signal to the goroutine that sends
// events to subscribers that there will be no more events and so it can stop.
func NewSimpleEventBus[T any]() *SimpleEventBus[T] {
	eventBus := &SimpleEventBus[T]{
		events:        make(chan T),
		subscriptions: make(map[*subscription[T]]struct{}),
	}
	go func() {
		for event := range eventBus.events {
			go func(event T) {
				eventBus.lock.RLock()
				defer eventBus.lock.RUnlock()
				for existingSubscription := range eventBus.subscriptions {
					go func(subscription *subscription[T]) {
						subscription.lock.RLock()
						defer subscription.lock.RUnlock()
						if subscription.stopped {
							return
						}
						subscription.wg.Go(func() {
							subscription.events <- event
						})
					}(existingSubscription)
				}
			}(event)
		}
	}()
	runtime.AddCleanup(eventBus, func(events chan T) {
		close(events)
	}, eventBus.events)

	return eventBus
}

// Publish sends a new event to the bus that will be broadcast to all subscribers.
//
// If the event bus is created using NewSimpleEventBus then this will only block until the goroutine that picks up
// events to send to subscribers is scheduled again, so unless there is resource starvation this should be effectively
// immediate.
func (s *SimpleEventBus[T]) Publish(event T) {
	s.events <- event
}

// Subscribe adds a new subscription to the event bus.
//
// This operation blocks until the subscription is registered, but unless there is resource starvation this should be
// effectively immediate.
//
// Once this function returns any new events will be received on the returned channel. Canclling the provided context
// will remove the subscription, events published shortly after context cancellation may still be received, no more
// events will be sent to the channel after the channel that is the second return parameter is closed (this parameter
// is intended to be used optionally by callers to check that no more events will be received).
//
// It is the caller's responsibility to drain all events from the returned channel (after the context is cancelled) to
// avoid leaking goroutines.
func (s *SimpleEventBus[T]) Subscribe(ctx context.Context) (<-chan T, <-chan struct{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	subscription := &subscription[T]{
		events: make(chan T),
	}
	s.subscriptions[subscription] = struct{}{}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		go func() {
			s.lock.Lock()
			defer s.lock.Unlock()
			delete(s.subscriptions, subscription)
		}()
		func() {
			subscription.lock.Lock()
			defer subscription.lock.Unlock()
			subscription.stopped = true
			close(done)
		}()
		subscription.wg.Wait()
		close(subscription.events)
	}()

	return subscription.events, done
}
