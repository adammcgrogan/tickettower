package ticketbot

import (
	"sync"
	"testing"
)

func TestKeyedLocksSerialiseAndForget(t *testing.T) {
	var k keyedLocks
	var wg sync.WaitGroup
	counter, max := 0, 0
	var cmu sync.Mutex
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := k.lock("guild:member")
			defer unlock()
			cmu.Lock()
			counter++
			if counter > max {
				max = counter
			}
			cmu.Unlock()
			cmu.Lock()
			counter--
			cmu.Unlock()
		}()
	}
	wg.Wait()
	if max != 1 {
		t.Errorf("%d goroutines held the same key at once", max)
	}
	if n := k.held(); n != 0 {
		t.Errorf("%d keys still held after every lock was released", n)
	}

	// Different keys don't block each other.
	u1 := k.lock("a")
	u2 := k.lock("b")
	if n := k.held(); n != 2 {
		t.Errorf("held = %d, want 2", n)
	}
	u1()
	u2()
	if n := k.held(); n != 0 {
		t.Errorf("held = %d after release, want 0", n)
	}
}
