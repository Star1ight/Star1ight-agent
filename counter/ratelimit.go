package counter

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	xrate "golang.org/x/time/rate"
)

const (
	defaultBurstWindow = 200 * time.Millisecond
	maxWriteChunk      = 64 * 1024
	maxReadChunk       = 64 * 1024
)

// RateLimiter is a byte-oriented token bucket shared by all connections for a
// node or a user. It uses x/time/rate so the hot path relies on the standard
// limiter implementation instead of a hand-rolled sleep/mutex loop.
type RateLimiter struct {
	mu       sync.RWMutex
	rate     int64
	burst    int
	limiter  *xrate.Limiter
	disabled bool
	closed   bool
}

func NewRateLimiter(bytesPerSecond int64) *RateLimiter {
	l := &RateLimiter{}
	l.SetRate(bytesPerSecond)
	return l
}

func burstFor(bytesPerSecond int64) int {
	burst := bytesPerSecond * int64(defaultBurstWindow) / int64(time.Second)
	if burst < maxWriteChunk {
		burst = maxWriteChunk
	}
	if burst > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(burst)
}

func (l *RateLimiter) SetRate(bytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rate = bytesPerSecond
	if bytesPerSecond <= 0 {
		l.disabled = true
		l.burst = 0
		l.limiter = nil
		return
	}
	burst := burstFor(bytesPerSecond)
	now := time.Now()
	if l.limiter == nil {
		l.limiter = xrate.NewLimiter(xrate.Limit(bytesPerSecond), burst)
	} else {
		l.limiter.SetLimitAt(now, xrate.Limit(bytesPerSecond))
		l.limiter.SetBurstAt(now, burst)
	}
	l.closed = false
	l.disabled = false
	l.burst = burst
	// x/time/rate starts full; draining the initial burst preserves the old
	// smoothing behavior where a fresh limiter did not allow an instant burst.
	l.limiter.AllowN(now, burst)
}

func (l *RateLimiter) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	l.disabled = true
	l.rate = 0
	l.burst = 0
	l.limiter = nil
}

func (l *RateLimiter) Closed() bool {
	if l == nil {
		return true
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closed
}

func (l *RateLimiter) Rate() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.rate
}

func (l *RateLimiter) snapshot() (*xrate.Limiter, int, bool, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.limiter, l.burst, l.disabled, l.closed
}

func (l *RateLimiter) Wait(n int) error {
	if l == nil || n <= 0 {
		return nil
	}
	remaining := n
	for remaining > 0 {
		limiter, burst, disabled, closed := l.snapshot()
		if closed {
			return ErrLimiterClosed
		}
		if disabled {
			return nil
		}
		chunk := remaining
		if burst > 0 && chunk > burst {
			chunk = burst
		}
		if limiter == nil {
			return nil
		}
		if err := limiter.WaitN(context.Background(), chunk); err != nil {
			return err
		}
		remaining -= chunk
	}
	return nil
}

var ErrLimiterClosed = errors.New("rate limiter closed")

// Allow reports whether n bytes can pass immediately. Unlike Wait, it never
// sleeps. Packet-based protocols such as QUIC/Hysteria2 run their own pacing,
// ACK, and congestion-control loops above the UDP socket; blocking the packet
// read/write path with time.Sleep can stall those loops and cause severe
// throughput oscillation. Allow is therefore used only by PacketConn wrappers
// to make a fast pass/drop decision while still charging accepted bytes to the
// shared limiter bucket.
func (l *RateLimiter) Allow(n int) (bool, error) {
	if l == nil || n <= 0 {
		return true, nil
	}
	limiter, burst, disabled, closed := l.snapshot()
	if closed {
		return false, ErrLimiterClosed
	}
	if disabled || limiter == nil {
		return true, nil
	}
	if burst > 0 && n > burst {
		n = burst
	}
	return limiter.AllowN(time.Now(), n), nil
}

type RateLimitedConn struct {
	N.ExtendedConn
	readLimiter  *RateLimiter
	writeLimiter *RateLimiter
}

func NewRateLimitedConn(conn net.Conn, readLimiter, writeLimiter *RateLimiter) net.Conn {
	if readLimiter == nil && writeLimiter == nil {
		return conn
	}
	return &RateLimitedConn{ExtendedConn: bufio.NewExtendedConn(conn), readLimiter: readLimiter, writeLimiter: writeLimiter}
}

func (c *RateLimitedConn) Read(b []byte) (int, error) {
	readBuf := b
	if c.readLimiter != nil && len(readBuf) > maxReadChunk {
		readBuf = readBuf[:maxReadChunk]
	}
	n, err := c.ExtendedConn.Read(readBuf)
	if n > 0 && c.readLimiter != nil {
		if waitErr := c.readLimiter.Wait(n); waitErr != nil && err == nil {
			err = waitErr
		}
	}
	return n, err
}

func (c *RateLimitedConn) Write(b []byte) (int, error) {
	if len(b) == 0 || c.writeLimiter == nil {
		return c.ExtendedConn.Write(b)
	}
	total := 0
	for total < len(b) {
		end := total + maxWriteChunk
		if end > len(b) {
			end = len(b)
		}
		chunk := b[total:end]
		if err := c.writeLimiter.Wait(len(chunk)); err != nil {
			return total, err
		}
		n, err := c.ExtendedConn.Write(chunk)
		total += n
		if err != nil {
			return total, err
		}
		if n != len(chunk) {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func (c *RateLimitedConn) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err == nil && buffer.Len() > 0 && c.readLimiter != nil {
		err = c.readLimiter.Wait(buffer.Len())
	}
	return err
}

func (c *RateLimitedConn) WriteBuffer(buffer *buf.Buffer) error {
	if buffer.Len() == 0 || c.writeLimiter == nil {
		return c.ExtendedConn.WriteBuffer(buffer)
	}
	for buffer.Len() > maxWriteChunk {
		if err := c.writeLimiter.Wait(maxWriteChunk); err != nil {
			return err
		}
		if _, err := c.ExtendedConn.Write(buffer.To(maxWriteChunk)); err != nil {
			return err
		}
		buffer.Advance(maxWriteChunk)
	}
	if buffer.Len() > 0 {
		if err := c.writeLimiter.Wait(buffer.Len()); err != nil {
			return err
		}
		return c.ExtendedConn.WriteBuffer(buffer)
	}
	return nil
}

func (c *RateLimitedConn) Upstream() any { return c.ExtendedConn }

type RateLimitedPacketConn struct {
	N.PacketConn
	readLimiter  *RateLimiter
	writeLimiter *RateLimiter
}

func NewRateLimitedPacketConn(conn N.PacketConn, readLimiter, writeLimiter *RateLimiter) N.PacketConn {
	if readLimiter == nil && writeLimiter == nil {
		return conn
	}
	return &RateLimitedPacketConn{PacketConn: conn, readLimiter: readLimiter, writeLimiter: writeLimiter}
}

func (p *RateLimitedPacketConn) ReadPacket(buff *buf.Buffer) (M.Socksaddr, error) {
	dest, err := p.PacketConn.ReadPacket(buff)
	if err == nil && buff.Len() > 0 && p.readLimiter != nil {
		if p.readLimiter.Closed() {
			err = ErrLimiterClosed
		}
	}
	return dest, err
}

func (p *RateLimitedPacketConn) WritePacket(buff *buf.Buffer, dest M.Socksaddr) error {
	if buff.Len() > 0 && p.writeLimiter != nil {
		if p.writeLimiter.Closed() {
			return ErrLimiterClosed
		}
	}
	return p.PacketConn.WritePacket(buff, dest)
}

func (p *RateLimitedPacketConn) Upstream() any { return p.PacketConn }

var _ io.Reader = (*RateLimitedConn)(nil)
