package eventbus_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alec-w/eventbus"
	"github.com/stretchr/testify/assert"
)

func wait(t *testing.T, timeoutMsg string, done <-chan struct{}) {
	t.Helper()
	ctx, cncl := context.WithTimeout(t.Context(), time.Second)
	defer cncl()
	select {
	case <-ctx.Done():
		t.Log(timeoutMsg)
		t.FailNow()
	case <-done:
	}
}

func TestEventBusEventReceived(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	// Act
	subscription, _ := bus.Subscribe(t.Context())
	event := rand.Text()
	bus.Publish(event)
	// Assert
	done := make(chan struct{})
	go func() {
		assert.Equal(t, event, <-subscription)
		close(done)
	}()
	wait(t, "subscription never received event", done)
}

func TestEventBusSubscriptionCancellation(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	ctx, cncl := context.WithCancel(t.Context())
	defer cncl()
	// Act
	subscription, _ := bus.Subscribe(ctx)
	event1 := rand.Text()
	bus.Publish(event1)
	// Assert
	done := make(chan struct{})
	go func() {
		assert.Equal(t, event1, <-subscription)
		close(done)
	}()
	wait(t, "subscription never received event", done)
	// Act
	cncl()
	// Assert
	done = make(chan struct{})
	go func() {
		_, ok := <-subscription
		assert.False(t, ok)
		close(done)
	}()
	wait(t, "subscription wasn't cancelled", done)
}

func TestEventBusEventSentToAllSubscribers(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	// Act
	subscription1, _ := bus.Subscribe(t.Context())
	subscription2, _ := bus.Subscribe(t.Context())
	event := rand.Text()
	bus.Publish(event)
	// Assert
	done1 := make(chan struct{})
	go func() {
		assert.Equal(t, event, <-subscription1)
		close(done1)
	}()
	done2 := make(chan struct{})
	go func() {
		assert.Equal(t, event, <-subscription2)
		close(done2)
	}()
	wait(t, "subscription 1 never received event", done1)
	wait(t, "subscription 2 never received event", done2)
}

func TestEventBusSubscriptionFiltersApplied(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	// Act
	subscription, _ := bus.Subscribe(
		t.Context(),
		func(i int) bool { return i%2 == 0 },
		func(i int) bool { return i%3 == 0 },
	)
	for i := range 13 {
		bus.Publish(i)
	}
	// Assert
	done := make(chan struct{})
	go func() {
		var receivedEvents []int
		expectedEvents := []int{0, 6, 12}
		receivedEvents = append(receivedEvents, <-subscription)
		receivedEvents = append(receivedEvents, <-subscription)
		receivedEvents = append(receivedEvents, <-subscription)
		assert.ElementsMatch(t, expectedEvents, receivedEvents)
		close(done)
	}()
	wait(t, "subscription did not receive all expected events", done)
}

func TestEventBusSubscribeNReceivesOnlyNEvents(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	limit := 2
	// Act
	subscription := bus.SubscribeN(t.Context(), 2)
	for i := range 10 {
		bus.Publish(i)
	}
	// Assert
	done := make(chan struct{})
	go func() {
		var receivedEvents []int
		for event := range subscription {
			receivedEvents = append(receivedEvents, event)
		}
		assert.Len(t, receivedEvents, limit)
		close(done)
	}()
	wait(t, "subscription was not cancelled", done)
}

func TestEventBusAwaitBlocksAndReceivesOnlyOneEvent(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	// Act
	done := make(chan struct{})
	event := 1
	go func() {
		// Assert
		assert.Equal(t, event, bus.Await(t.Context()))
		close(done)
	}()
eventSendLoop:
	for {
		timeOutTicker := time.Tick(time.Second)
		eventSendTicker := time.Tick(10 * time.Millisecond)
		select {
		case <-timeOutTicker:
			t.Log("Sending events for > 1 second")
			t.FailNow()
		case <-eventSendTicker:
			bus.Publish(event)
		case <-done:
			break eventSendLoop
		}
	}
	wait(t, "event was never received", done)
}

func TestEventBusRegisterHandlesEvents(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	count := 0
	out := make(chan struct {
		event string
		index int
	})
	// Act
	bus.Register(t.Context(), func(_ context.Context, event string) error {
		out <- struct {
			event string
			index int
		}{event: event, index: count}
		count++
		return nil
	})
	event := rand.Text()
	bus.Publish(event)
	bus.Publish(event)
	// Assert
	done := make(chan struct{})
	go func() {
		handledEvent1 := <-out
		assert.Equal(t, event, handledEvent1.event)
		assert.Equal(t, 0, handledEvent1.index)
		handledEvent2 := <-out
		assert.Equal(t, event, handledEvent2.event)
		assert.Equal(t, 1, handledEvent2.index)
		close(done)
	}()
	wait(t, "handler never processed event", done)
}

func TestEventBusRegisterReturnsErrors(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	expectedErr := errors.New(rand.Text())
	// Act
	errs := bus.Register(t.Context(), func(_ context.Context, _ string) error {
		return expectedErr
	})
	bus.Publish(rand.Text())
	// Assert
	done := make(chan struct{})
	go func() {
		assert.Equal(t, expectedErr, <-errs)
		close(done)
	}()
	wait(t, "handler never processed event", done)
}

func TestEventBusRegisterNHandlesOnlyLimitEvents(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	out := make(chan struct {
		event int
		index int
	})
	limit := 2
	counter := 0
	// Act
	errs := bus.RegisterN(t.Context(), func(_ context.Context, event int) error {
		out <- struct {
			event int
			index int
		}{event: event, index: counter}
		counter++
		return nil
	}, limit)
	for i := range 10 {
		bus.Publish(i + 1)
	}
	// Assert
	done := make(chan struct{})
	go func() {
		<-errs
		close(out)
	}()
	go func() {
		var handledEvents []struct {
			event int
			index int
		}
		for handledEvent := range out {
			handledEvents = append(handledEvents, handledEvent)
		}
		if assert.Len(t, handledEvents, limit) {
			for i, handledEvent := range handledEvents {
				assert.Equal(t, i, handledEvent.index)
			}
		}
		close(done)
	}()
	wait(t, "handler was never unregistered", done)
}

func TestEventBusRegisterNReturnsErrors(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	limit := 2
	// Act
	errs := bus.RegisterN(t.Context(), func(_ context.Context, event int) error {
		return fmt.Errorf("%d", event)
	}, limit)
	for i := range 10 {
		bus.Publish(i)
	}
	// Assert
	done := make(chan struct{})
	go func() {
		var handledErrs []error
		for err := range errs {
			handledErrs = append(handledErrs, err)
		}
		assert.Len(t, handledErrs, limit)
		close(done)
	}()
	wait(t, "handler was never unregistered", done)
}

func TestEventBusRegisterAwaitBlocksAndHandlesOnlyOneEvent(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	var out int
	done := make(chan struct{})
	// Act
	go func() {
		assert.NoError(
			t,
			bus.RegisterAwait(t.Context(), func(_ context.Context, event int) error {
				out = event
				return nil
			}),
		)
		close(done)
	}()
	for i := range 10 {
		bus.Publish(i + 1)
	}
	wait(t, "handler never returned", done)
	// Assert
	assert.Positive(t, out)
}

func TestEventBusRegisterAwaitBlocksAndReturnsError(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	expectedErr := errors.New(rand.Text())
	var err error
	done := make(chan struct{})
	// Act
	go func() {
		err = bus.RegisterAwait(t.Context(), func(_ context.Context, _ int) error {
			return expectedErr
		})
		close(done)
	}()
	for i := range 10 {
		bus.Publish(i + 1)
	}
	wait(t, "handler never returned", done)
	// Assert
	assert.Equal(t, expectedErr, err)
}

func TestEventBusRegisterAsyncHandlesEvents(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	out := make(chan string)
	// Act
	bus.RegisterAsync(t.Context(), func(_ context.Context, event string) error {
		out <- event
		return nil
	})
	event1 := rand.Text()
	bus.Publish(event1)
	event2 := rand.Text()
	bus.Publish(event2)
	// Assert
	done := make(chan struct{})
	go func() {
		handledEvents := make([]string, 0)
		handledEvents = append(handledEvents, <-out)
		handledEvents = append(handledEvents, <-out)
		assert.ElementsMatch(t, []string{event1, event2}, handledEvents)
		close(done)
	}()
	wait(t, "handler never processed events", done)
}

func TestEventBusRegisterAsyncReturnsErrors(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[string]()
	expectedErr1 := errors.New(rand.Text())
	expectedErr2 := errors.New(rand.Text())
	expectedErrs := make(chan error, 2)
	expectedErrs <- expectedErr1
	expectedErrs <- expectedErr2
	// Act
	errs := bus.RegisterAsync(
		t.Context(),
		func(_ context.Context, _ string) error {
			return <-expectedErrs
		},
	)
	bus.Publish(rand.Text())
	bus.Publish(rand.Text())
	// Assert
	done := make(chan struct{})
	go func() {
		handledErrs := make([]error, 0)
		handledErrs = append(handledErrs, <-errs)
		handledErrs = append(handledErrs, <-errs)
		assert.ElementsMatch(t, []error{expectedErr1, expectedErr2}, handledErrs)
		close(done)
	}()
	wait(t, "handler never processed events", done)
}

func TestEventBusRegisterAsyncNHandlesEvents(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	out := make(chan int)
	limit := 2
	// Act
	errs := bus.RegisterAsyncN(t.Context(), func(_ context.Context, event int) error {
		out <- event
		return nil
	}, limit)
	for i := range 10 {
		bus.Publish(i)
	}
	// Assert
	done := make(chan struct{})
	go func() {
		<-errs
		close(out)
	}()
	go func() {
		handledEvents := make([]int, 0)
		for event := range out {
			handledEvents = append(handledEvents, event)
		}
		assert.Len(t, handledEvents, limit)
		close(done)
	}()
	wait(t, "handler never processed events", done)
}

func TestEventBusRegisterAsyncNReturnsErrors(t *testing.T) {
	t.Parallel()
	// Arrange
	bus := eventbus.New[int]()
	expectedErr1 := errors.New(rand.Text())
	expectedErr2 := errors.New(rand.Text())
	expectedErrs := make(chan error, 2)
	expectedErrs <- expectedErr1
	expectedErrs <- expectedErr2
	limit := 2
	// Act
	errs := bus.RegisterAsyncN(t.Context(), func(_ context.Context, _ int) error {
		return <-expectedErrs
	}, limit)
	for i := range 10 {
		bus.Publish(i)
	}
	// Assert
	done := make(chan struct{})
	go func() {
		handledErrs := make([]error, 0)
		for err := range errs {
			handledErrs = append(handledErrs, err)
		}
		assert.ElementsMatch(t, []error{expectedErr1, expectedErr2}, handledErrs)
		close(done)
	}()
	wait(t, "handler never processed events", done)
}
