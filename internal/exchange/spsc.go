package exchange

import (
	"bytes"
	"sync/atomic"
	"unsafe"
)

type node struct {
	data *bytes.Buffer
	next *node
}

// SPSC is a lock-free queue for []byte (single producer, single consumer), used to recycle marketdata multicast packets
type SPSC struct {
	head *node
}

func (q *SPSC) put(data *bytes.Buffer) {
	n := &node{}
	n.data = data
	for {
		n.next = (*node)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))))
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(n.next), unsafe.Pointer(n)) {
			return
		}
	}
}

func (q *SPSC) get() *bytes.Buffer {
	for {
		head := (*node)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))))

		if head == nil {
			return nil
		}

		next := head.next

		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(head), unsafe.Pointer(next)) {
			return head.data
		}
	}
}
