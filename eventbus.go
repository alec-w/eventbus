package eventbus

import (
	"context"
	"runtime"
	"sync"
)

// Filter is used to filter out events.
//
// Custom logic can be implemented for each filter, but it must not panic and must be able to run concurrently with
// other filters.
type Filter[T any] func(T) bool

type subscription[T any] struct {
	events  chan T
	wg      sync.WaitGroup
	lock    sync.RWMutex
	stopped bool
}

// EventBus is an implementation of an event bus that supports subscription cancellation, event filtering and handler
// registration.
type EventBus[T any] struct {
	events        chan T
	subscriptions map[*subscription[T]]struct{}
	lock          sync.RWMutex
}

// New instantiates a new EventBus for the given event type.
//
// A runtime cleanup function is registered to close the events channel and thereby signal to the goroutine that sends
// events to subscribers that there will be no more events and so it can stop.
func New[T any]() *EventBus[T] {
	eventBus := &EventBus[T]{
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
func (e *EventBus[T]) Publish(event T) {
	e.events <- event
}

// Subscribe adds a new subscription to the event bus.
//
// Filters can be provided that exclude events from being sent to the output subscription channel. All filters must
// return true for an event to be included. Filters are run sequentially for each event in the order they are supplied.
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
func (e *EventBus[T]) Subscribe(
	ctx context.Context,
	filters ...Filter[T],
) (<-chan T, <-chan struct{}) {
	e.lock.Lock()
	defer e.lock.Unlock()
	subscription := &subscription[T]{
		events: make(chan T),
	}
	e.subscriptions[subscription] = struct {
	}{}
	done := make(chan struct{})
	filteredEvents := make(chan T)
	go func() {
	eventsLoop:
		for event := range subscription.events {
			for _, filter := range filters {
				if !filter(event) {
					continue eventsLoop
				}
			}
			filteredEvents <- event
		}
		close(filteredEvents)
	}()
	go func() {
		<-ctx.Done()
		go func() {
			e.lock.Lock()
			defer e.lock.Unlock()
			delete(e.subscriptions, subscription)
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

	return filteredEvents, done
}

// SubscribeN behaves the same as Subscribe but will return at most limit number of items, after which the
// subscription will be removed (cancelling the context early will terminate the subscription before the limit is
// reached).
func (e *EventBus[T]) SubscribeN(
	ctx context.Context,
	limit int,
	filters ...Filter[T],
) <-chan T {
	ctx, cncl := context.WithCancel(ctx)
	subscription, _ := e.Subscribe(ctx, filters...)
	output := make(chan T)
	go func() {
		var count int
		for event := range subscription {
			if count == limit {
				continue
			}
			count++
			output <- event
			if count == limit {
				cncl()
			}
		}
		close(output)
	}()

	return output
}

// Await is a blocking shorthand for SubscribeN with a limit of 1.
//
// This blocks until an event is received matching the supplied filters which is then returned.
func (e *EventBus[T]) Await(ctx context.Context, filters ...Filter[T]) T {
	return <-e.SubscribeN(ctx, 1, filters...)
}

// register attaches a handler to a subscription. Events are handled serially. Errors from handling events are returned
// to the caller.
//
// errors from events may be returned out of order, it is the caller's responsibility to add enough information to
// errors to reorder them later if needed.
//
// Cancelling the supplied context will cancel the underlying subscription, the error channel will be closed once the
// last event has been handled.
func (e *EventBus[T]) register(
	ctx context.Context,
	subscription <-chan T,
	handler func(context.Context, T) error,
) <-chan error {
	errs := make(chan error)
	go func() {
		var wg sync.WaitGroup
		for event := range subscription {
			if err := handler(ctx, event); err != nil {
				wg.Go(func() {
					errs <- err
				})
			}
		}
		wg.Wait()
		close(errs)
	}()

	return errs
}

// register attaches a handler to a subscription. Events are handled concurrently, so the handler must support
// concurrent execution. Errors from handling events are returned to the caller.
//
// Cancelling the supplied context will cancel the underlying subscription, the error channel will be closed once all
// events have been handled.
func (e *EventBus[T]) registerAsync(
	ctx context.Context,
	subscription <-chan T,
	handler func(context.Context, T) error,
) <-chan error {
	errs := make(chan error)
	go func() {
		var wg sync.WaitGroup
		for event := range subscription {
			wg.Go(func() {
				if err := handler(ctx, event); err != nil {
					errs <- err
				}
			})
		}
		wg.Wait()
		close(errs)
	}()

	return errs
}

// Register subscribes with the given filters and context, then runs for provided handler for each event.
//
// The handler is run for each event serially, so the handler must not block based on the output of other calls to the
// same registered handler.
//
// Any errors from the handler are pushed to the returned error channel. These may be out of order, it is the caller's#
// responsibility to include enough information in the errors to reorder them later if that is required.
func (e *EventBus[T]) Register(
	ctx context.Context,
	handler func(context.Context, T) error,
	filters ...Filter[T],
) <-chan error {
	subscription, _ := e.Subscribe(ctx, filters...)

	return e.register(ctx, subscription, handler)
}

// RegisterN is the same as Register, but the handler will be called a maximum of limit times, after which the handler
// will be removed.
func (e *EventBus[T]) RegisterN(
	ctx context.Context,
	handler func(context.Context, T) error,
	limit int,
	filters ...Filter[T],
) <-chan error {
	return e.register(ctx, e.SubscribeN(ctx, limit, filters...), handler)
}

// RegisterAwait is a blocking shorthand for RegisterN with limit 1.
//
// This blocks until an event is handled matching the supplied filters.
func (e *EventBus[T]) RegisterAwait(
	ctx context.Context,
	handler func(context.Context, T) error,
	filters ...Filter[T],
) error {
	return <-e.RegisterN(ctx, handler, 1, filters...)
}

// RegisterAsync is like Register but the handler calls run concurrently, so events may be processed out of order.
func (e *EventBus[T]) RegisterAsync(
	ctx context.Context,
	handler func(context.Context, T) error,
	filters ...Filter[T],
) <-chan error {
	subscription, _ := e.Subscribe(ctx, filters...)

	return e.registerAsync(ctx, subscription, handler)
}

// RegisterAsyncN is like RegisterN but the handler calls run concurrently, so events may be processed out of order.
func (e *EventBus[T]) RegisterAsyncN(
	ctx context.Context,
	handler func(context.Context, T) error,
	limit int,
	filters ...Filter[T],
) <-chan error {
	return e.registerAsync(ctx, e.SubscribeN(ctx, limit, filters...), handler)
}
