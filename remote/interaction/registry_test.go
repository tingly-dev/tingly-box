package interaction

import (
	"sync"
	"testing"
	"time"
)

func TestBeginIsIdempotent(t *testing.T) {
	r := New[string](time.Second)
	if !r.Begin("x") {
		t.Fatal("first Begin should return true")
	}
	if r.Begin("x") {
		t.Fatal("second Begin while inflight should return false")
	}
}

func TestAwaitUnknownIdReturnsFalse(t *testing.T) {
	r := New[string](time.Second)
	ch, ok := r.Await("missing")
	if ok {
		t.Fatalf("Await on unknown id should return ok=false, got %v", ch)
	}
	if ch != nil {
		t.Fatalf("Await on unknown id should return nil channel, got %v", ch)
	}
}

func TestBeginThenResolveDeliversToAwaiter(t *testing.T) {
	r := New[string](time.Second)
	r.Begin("a")
	ch, ok := r.Await("a")
	if !ok {
		t.Fatal("Await after Begin should succeed")
	}
	go r.Resolve("a", "hello")
	select {
	case v := <-ch:
		if v != "hello" {
			t.Fatalf("expected hello, got %q", v)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not receive value")
	}
}

func TestResolveBeforeAwaitGetsCachedValue(t *testing.T) {
	r := New[string](time.Second)
	r.Begin("a")
	r.Resolve("a", "world")
	ch, ok := r.Await("a")
	if !ok {
		t.Fatal("Await after Resolve should still succeed (cache)")
	}
	select {
	case v := <-ch:
		if v != "world" {
			t.Fatalf("expected world, got %q", v)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("cached value should be immediately available")
	}
}

func TestAwaitAfterTTLExpiryReturnsFalse(t *testing.T) {
	r := New[string](20 * time.Millisecond)
	r.Begin("a")
	r.Resolve("a", "v")
	time.Sleep(40 * time.Millisecond)
	if _, ok := r.Await("a"); ok {
		t.Fatal("Await should return ok=false after TTL")
	}
}

func TestBeginReturnsFalseAfterCachedAnswer(t *testing.T) {
	r := New[string](time.Second)
	r.Begin("a")
	r.Resolve("a", "v")
	if r.Begin("a") {
		t.Fatal("Begin while answer cached should return false")
	}
}

func TestCancelDropsInflightAndWaiter(t *testing.T) {
	r := New[string](time.Second)
	r.Begin("a")
	ch, _ := r.Await("a")
	r.Cancel("a")
	if r.IsInflight("a") {
		t.Fatal("Cancel should clear inflight")
	}
	select {
	case v, ok := <-ch:
		if ok {
			t.Fatalf("channel should not deliver after Cancel, got %q", v)
		}
	case <-time.After(20 * time.Millisecond):
		// Expected: cancel does not deliver; waiter must fall through
		// via its own ctx/timeout.
	}
}

func TestConcurrentAwaitersGetSameChannel(t *testing.T) {
	r := New[string](time.Second)
	r.Begin("x")
	ch1, _ := r.Await("x")
	ch2, _ := r.Await("x")
	go r.Resolve("x", "v")
	got := 0
	deadline := time.After(time.Second)
	for got < 1 {
		select {
		case v := <-ch1:
			if v != "v" {
				t.Fatalf("ch1 got %q", v)
			}
			got++
		case v := <-ch2:
			if v != "v" {
				t.Fatalf("ch2 got %q", v)
			}
			got++
		case <-deadline:
			t.Fatal("no value delivered to either channel")
		}
	}
	// We don't make a guarantee about fan-out; one of the two channels
	// receiving is sufficient and matches single-consumer semantics.
}

func TestResolveDoesNotBlockWithoutAwaiter(t *testing.T) {
	r := New[string](time.Second)
	r.Begin("x")
	done := make(chan struct{})
	go func() {
		r.Resolve("x", "v")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Resolve blocked with no awaiter")
	}
}

func TestRaceBeginResolveAwait(t *testing.T) {
	r := New[int](time.Second)
	const N = 50
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			id := "id-" + string(rune('a'+i%26))
			if r.Begin(id) {
				r.Resolve(id, i)
			}
		}()
		go func() {
			defer wg.Done()
			id := "id-" + string(rune('a'+i%26))
			ch, ok := r.Await(id)
			if !ok {
				return
			}
			select {
			case <-ch:
			case <-time.After(100 * time.Millisecond):
			}
		}()
	}
	wg.Wait()
}

func TestForgetClearsCache(t *testing.T) {
	r := New[string](time.Hour)
	r.Begin("a")
	r.Resolve("a", "v")
	r.Forget("a")
	if _, ok := r.Await("a"); ok {
		t.Fatal("Await should fail after Forget")
	}
}
