package wireguard

import (
	"net"
	"os"
	"sync/atomic"

	"golang.zx2c4.com/wireguard/tun"
)

// MARK: File
// Returns the underlying file for the TUN device
func (w *TUNWrapper) File() *os.File {
	return nil
}

// MARK: Read
// Reads data from the TUN interface into provided buffers
func (w *TUNWrapper) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if atomic.LoadInt64(&w.closed) != 0 || len(bufs) == 0 {
		return 0, net.ErrClosed
	}

	n, err := w.iface.Read(bufs[0][offset:])
	if n > 0 {
		sizes[0] = n
		return 1, nil
	}

	return 0, err
}

// MARK: Write
// Writes data from provided buffers to the TUN interface
func (w *TUNWrapper) Write(bufs [][]byte, offset int) (int, error) {
	if atomic.LoadInt64(&w.closed) != 0 {
		return 0, net.ErrClosed
	}

	count := 0
	for _, buf := range bufs {
		if len(buf) <= offset {
			continue
		}

		_, err := w.iface.Write(buf[offset:])
		if err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// MARK: Flush
// Flushes buffered data (no-op implementation)
func (w *TUNWrapper) Flush() error {
	return nil
}

// MARK: MTU
// Returns the MTU of the TUN interface
func (w *TUNWrapper) MTU() (int, error) {
	if w.mtu <= 0 {
		return 1420, nil
	}
	return w.mtu, nil
}

// MARK: Name
// Returns the name of the TUN interface
func (w *TUNWrapper) Name() (string, error) {
	return w.name, nil
}

// MARK: Events
// Returns a channel for TUN events
func (w *TUNWrapper) Events() <-chan tun.Event {
	return w.events
}

// MARK: Close
// Closes the TUN interface and stops all associated operations
func (w *TUNWrapper) Close() error {
	if !atomic.CompareAndSwapInt64(&w.closed, 0, 1) {
		return nil
	}

	if w.cancel != nil {
		w.cancel()
	}

	if w.events != nil {
		close(w.events)
	}

	w.wg.Wait()

	if w.iface != nil {
		return w.iface.Close()
	}

	return nil
}

// MARK: BatchSize
// Returns the configured batch size for reads/writes
func (w *TUNWrapper) BatchSize() int {
	return w.batchSize
}
