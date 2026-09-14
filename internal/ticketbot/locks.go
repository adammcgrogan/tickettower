package ticketbot

import "sync"

// keyedLocks hands out a mutex per key and forgets the key once nobody holds
// or waits for it, so one lock per member who ever opened a ticket doesn't
// stay in memory for the life of the process.
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	mu    sync.Mutex
	users int // holders and waiters
}

// lock takes the key's lock and returns the function that releases it.
func (k *keyedLocks) lock(key string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*keyedLock)
	}
	l, ok := k.locks[key]
	if !ok {
		l = &keyedLock{}
		k.locks[key] = l
	}
	l.users++
	k.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		k.mu.Lock()
		l.users--
		if l.users == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}

// held is how many keys currently have a holder or waiter.
func (k *keyedLocks) held() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.locks)
}
