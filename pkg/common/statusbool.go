package common

import (
	"sync/atomic"
	"time"
)

type StatusBool struct {
	b uint32
}

// simple go routine synchronization with timeout.
// the status boolean is not in the critical path, so performance is not a concern
// but it was a much simpler way to sync go routines with a timeout, the alternative using
// channels seems to allow the go routine to leak, if the reader timeouts before the
// sender writes

func (sb *StatusBool) WaitForTrue(timeoutMS int64) bool {
	expires := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for atomic.LoadUint32(&sb.b) == 0 {
		time.Sleep(100 * time.Millisecond)
		if time.Now().After(expires) {
			return false
		}
	}
	return true
}
func (sb *StatusBool) WaitForFalse(timeoutMS int64) bool {
	expires := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for atomic.LoadUint32(&sb.b) != 0 {
		time.Sleep(100 * time.Millisecond)
		if time.Now().After(expires) {
			return false
		}
	}
	return true
}

func (sb *StatusBool) SetTrue() {
	atomic.StoreUint32(&sb.b, 1)
}
func (sb *StatusBool) SetFalse() {
	atomic.StoreUint32(&sb.b, 0)
}
func (sb *StatusBool) IsTrue() bool {
	if atomic.LoadUint32(&sb.b) != 0 {
		return true
	} else {
		return false
	}
}
