package main

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type trackedFastPathConn struct{}

func (c *trackedFastPathConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *trackedFastPathConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *trackedFastPathConn) Close() error                     { return nil }
func (c *trackedFastPathConn) LocalAddr() net.Addr              { return trackedDummyAddr("local") }
func (c *trackedFastPathConn) RemoteAddr() net.Addr             { return trackedDummyAddr("remote") }
func (c *trackedFastPathConn) SetDeadline(time.Time) error      { return nil }
func (c *trackedFastPathConn) SetReadDeadline(time.Time) error  { return nil }
func (c *trackedFastPathConn) SetWriteDeadline(time.Time) error { return nil }

func (c *trackedFastPathConn) CreateReadWaiter() (N.ReadWaiter, bool) {
	return trackedNoopReadWaiter{}, true
}

type trackedNoopReadWaiter struct{}

func (trackedNoopReadWaiter) InitializeReadWaiter(N.ReadWaitOptions) bool { return false }
func (trackedNoopReadWaiter) WaitReadBuffer() (*buf.Buffer, error)        { return nil, io.EOF }

type trackedPacketFastPathConn struct{}

func (c *trackedPacketFastPathConn) ReadPacket(*buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.EOF
}
func (c *trackedPacketFastPathConn) WritePacket(*buf.Buffer, M.Socksaddr) error {
	return nil
}
func (c *trackedPacketFastPathConn) Close() error                     { return nil }
func (c *trackedPacketFastPathConn) LocalAddr() net.Addr              { return trackedDummyAddr("packet") }
func (c *trackedPacketFastPathConn) SetDeadline(time.Time) error      { return nil }
func (c *trackedPacketFastPathConn) SetReadDeadline(time.Time) error  { return nil }
func (c *trackedPacketFastPathConn) SetWriteDeadline(time.Time) error { return nil }

type trackedDummyAddr string

func (a trackedDummyAddr) Network() string { return "test" }
func (a trackedDummyAddr) String() string  { return string(a) }

func TestTrackedConnKeepsReadWaiterFastPath(t *testing.T) {
	base := &trackedFastPathConn{}
	wrapped := &trackedConn{Conn: base, release: func() {}}

	if _, ok := bufio.CreateReadWaiter(wrapped); !ok {
		t.Fatal("trackedConn hid the upstream read waiter fast path")
	}
}

func TestTrackedPacketConnIsReplaceable(t *testing.T) {
	base := &trackedPacketFastPathConn{}
	wrapped := &trackedPacketConn{PacketConn: base, release: func() {}}

	if got := N.UnwrapPacketReader(wrapped); got != base {
		t.Fatalf("trackedPacketConn reader unwrapped to %T, want upstream packet conn", got)
	}
	if got := N.UnwrapPacketWriter(wrapped); got != base {
		t.Fatalf("trackedPacketConn writer unwrapped to %T, want upstream packet conn", got)
	}
}

var _ net.Conn = (*trackedFastPathConn)(nil)
var _ N.ReadWaitCreator = (*trackedFastPathConn)(nil)
var _ N.PacketConn = (*trackedPacketFastPathConn)(nil)
