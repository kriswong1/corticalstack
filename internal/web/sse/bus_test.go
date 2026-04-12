package sse

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFormatSSE(t *testing.T) {
	t.Run("simple event", func(t *testing.T) {
		ev := Event{Type: "job_status", Data: map[string]string{"id": "abc", "msg": "hi"}}
		got, err := FormatSSE(ev)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s := string(got)
		if !strings.HasPrefix(s, "event: job_status\n") {
			t.Errorf("missing event header: %q", s)
		}
		if !strings.Contains(s, "data: ") {
			t.Errorf("missing data line: %q", s)
		}
		if !strings.HasSuffix(s, "\n\n") {
			t.Errorf("missing trailing blank line: %q", s)
		}
		if !strings.Contains(s, `"id":"abc"`) || !strings.Contains(s, `"msg":"hi"`) {
			t.Errorf("data payload missing fields: %q", s)
		}
	})

	t.Run("unserializable data errors", func(t *testing.T) {
		// channels are not JSON-serializable
		ev := Event{Type: "bad", Data: make(chan int)}
		_, err := FormatSSE(ev)
		if err == nil {
			t.Error("expected error for unserializable data")
		}
	})
}

func TestEventBusPublishDelivers(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	bus.Publish(Event{Type: "a", Data: 1})
	bus.Publish(Event{Type: "b", Data: 2})
	bus.Publish(Event{Type: "c", Data: 3})

	// All three events should be immediately available in order.
	want := []string{"a", "b", "c"}
	for i, w := range want {
		select {
		case ev := <-ch:
			if ev.Type != w {
				t.Errorf("[%d] got %q, want %q", i, ev.Type, w)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("[%d] timed out waiting for event %q", i, w)
		}
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()
	defer bus.Unsubscribe(ch1)
	defer bus.Unsubscribe(ch2)

	bus.Publish(Event{Type: "fanout"})

	for _, ch := range []chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != "fanout" {
				t.Errorf("wrong event type: %q", ev.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("subscriber did not receive fanout event")
		}
	}
}

func TestEventBusUnsubscribeClosesChannel(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	bus.Unsubscribe(ch)

	// Reading from a closed channel returns the zero value + ok=false.
	_, ok := <-ch
	if ok {
		t.Error("expected closed channel after Unsubscribe")
	}
}

func TestEventBusUnsubscribeIdempotent(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	bus.Unsubscribe(ch)
	// Second call should not panic and should be a no-op.
	bus.Unsubscribe(ch)
}

func TestEventBusDropsWhenSubscriberFull(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	// Publish more than the 64-event buffer; extras should be dropped,
	// not block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			bus.Publish(Event{Type: "flood"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked instead of dropping")
	}

	// Drain and count. Should be at most the buffer size (64).
	received := 0
drain:
	for {
		select {
		case <-ch:
			received++
		case <-time.After(50 * time.Millisecond):
			break drain
		}
	}
	if received == 0 || received > 64 {
		t.Errorf("received %d events, want 1..64", received)
	}
}

// TestEventBusConcurrent exercises Subscribe/Publish/Unsubscribe from
// multiple goroutines under -race.
func TestEventBusConcurrent(t *testing.T) {
	bus := NewEventBus()
	var wg sync.WaitGroup

	// Publisher
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				bus.Publish(Event{Type: "tick"})
			}
		}
	}()

	// Subscribers that churn in and out.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ch := bus.Subscribe()
				select {
				case <-ch:
				case <-time.After(5 * time.Millisecond):
				}
				bus.Unsubscribe(ch)
			}
		}()
	}

	// Let subscribers run, then stop publisher.
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
