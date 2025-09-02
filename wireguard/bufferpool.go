package wireguard

import (
	"sync/atomic"
	"unsafe"
)

// MARK: NewPacketBufferPool
// Creates a new packet buffer pool with a maximum size
func NewPacketBufferPool(maxSize int) *PacketBufferPool {
	return &PacketBufferPool{
		max: int32(maxSize),
	}
}

// MARK: Get (PacketBufferPool)
// Retrieves a PacketBuffer from the pool or allocates a new one if empty
func (p *PacketBufferPool) Get() *PacketBuffer {
	for {
		head := atomic.LoadPointer(&p.head)
		if head == nil {
			return &PacketBuffer{
				data: make([]byte, 2048),
			}
		}

		buf := (*PacketBuffer)(head)
		next := atomic.LoadPointer(&buf.next)

		if atomic.CompareAndSwapPointer(&p.head, head, next) {
			atomic.AddInt32(&p.size, -1)
			buf.next = nil
			buf.length = 0
			return buf
		}
	}
}

// MARK: Put (PacketBufferPool)
// Returns a PacketBuffer to the pool if pool is not full
func (p *PacketBufferPool) Put(buf *PacketBuffer) {
	if buf == nil {
		return
	}

	if atomic.LoadInt32(&p.size) >= p.max {
		return
	}

	for {
		head := atomic.LoadPointer(&p.head)
		buf.next = head

		if atomic.CompareAndSwapPointer(&p.head, head, unsafe.Pointer(buf)) {
			atomic.AddInt32(&p.size, 1)
			return
		}
	}
}
