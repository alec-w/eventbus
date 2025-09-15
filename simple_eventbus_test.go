package eventbus_test

import (
	"context"
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

func TestSimpleEventBusEventReceived(t *testing.T) {
	t.Parallel()
	simpleEventBus := eventbus.NewSimpleEventBus[string]()
	subscription, _ := simpleEventBus.Subscribe(t.Context())
	event := "test"
	simpleEventBus.Publish(event)
	done := make(chan struct{})
	go func() {
		assert.Equal(t, event, <-subscription)
		close(done)
	}()
	wait(t, "subscription never received event", done)
}

func TestSimpleEventBusSubscriptionCancellation(t *testing.T) {
	t.Parallel()
	simpleEventBus := eventbus.NewSimpleEventBus[string]()
	ctx, cncl := context.WithCancel(t.Context())
	defer cncl()
	subscription, subscriptionStopped := simpleEventBus.Subscribe(ctx)
	event1 := "test1"
	simpleEventBus.Publish(event1)
	done := make(chan struct{})
	go func() {
		assert.Equal(t, event1, <-subscription)
		close(done)
	}()
	wait(t, "subscription never received event", done)
	cncl()
	wait(t, "subscription never stopped", subscriptionStopped)
	simpleEventBus.Publish("test2")
	done = make(chan struct{})
	go func() {
		_, ok := <-subscription
		assert.False(t, ok)
		close(done)
	}()
	wait(t, "subscription wasn't cancelled", done)
}

func TestSimpleEventBusEventSentToAllSubscribers(t *testing.T) {
	t.Parallel()
	simpleEventBus := eventbus.NewSimpleEventBus[string]()
	subscription1, _ := simpleEventBus.Subscribe(t.Context())
	subscription2, _ := simpleEventBus.Subscribe(t.Context())
	event := "test"
	simpleEventBus.Publish(event)
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
