package main

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type headroomPacketConn struct {
	front int
	rear  int
}

func (c *headroomPacketConn) ReadPacket(*buf.Buffer) (M.Socksaddr, error) {
	return M.Socksaddr{}, io.EOF
}

func (c *headroomPacketConn) WritePacket(*buf.Buffer, M.Socksaddr) error {
	return nil
}

func (c *headroomPacketConn) Close() error {
	return nil
}

func (c *headroomPacketConn) LocalAddr() net.Addr {
	return dummyPacketAddr("local")
}

func (c *headroomPacketConn) SetDeadline(time.Time) error {
	return nil
}

func (c *headroomPacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *headroomPacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

func (c *headroomPacketConn) FrontHeadroom() int {
	return c.front
}

func (c *headroomPacketConn) RearHeadroom() int {
	return c.rear
}

type dummyPacketAddr string

func (a dummyPacketAddr) Network() string { return "test" }
func (a dummyPacketAddr) String() string  { return string(a) }

func TestTrackedPacketConnPreservesUnderlyingHeadroom(t *testing.T) {
	base := &headroomPacketConn{front: 512, rear: 32}
	wrapped := &trackedPacketConn{PacketConn: base, release: func() {}}

	if got := N.CalculateFrontHeadroom(wrapped); got != base.front {
		t.Fatalf("front headroom through trackedPacketConn = %d, want %d", got, base.front)
	}
	if got := N.CalculateRearHeadroom(wrapped); got != base.rear {
		t.Fatalf("rear headroom through trackedPacketConn = %d, want %d", got, base.rear)
	}
}
