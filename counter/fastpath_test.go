package counter

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	N "github.com/sagernet/sing/common/network"
)

type fastPathConn struct {
	vectorisedWrites int
	vectorisedBytes  int
}

func (c *fastPathConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *fastPathConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *fastPathConn) Close() error                     { return nil }
func (c *fastPathConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c *fastPathConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c *fastPathConn) SetDeadline(time.Time) error      { return nil }
func (c *fastPathConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fastPathConn) SetWriteDeadline(time.Time) error { return nil }

func (c *fastPathConn) CreateReadWaiter() (N.ReadWaiter, bool) {
	return noopReadWaiter{}, true
}

func (c *fastPathConn) WriteVectorised(buffers []*buf.Buffer) error {
	defer buf.ReleaseMulti(buffers)
	c.vectorisedWrites++
	c.vectorisedBytes += buf.LenMulti(buffers)
	return nil
}

type noopReadWaiter struct{}

func (noopReadWaiter) InitializeReadWaiter(N.ReadWaitOptions) bool { return false }
func (noopReadWaiter) WaitReadBuffer() (*buf.Buffer, error)        { return nil, io.EOF }

type dummyAddr string

func (a dummyAddr) Network() string { return "test" }
func (a dummyAddr) String() string  { return string(a) }

func TestConnCounterKeepsReadWaiterAndUnwrapsCounter(t *testing.T) {
	base := &fastPathConn{}
	storage := &TrafficStorage{}
	wrapped := NewConnCounter(base, storage)

	if _, ok := bufio.CreateReadWaiter(wrapped); !ok {
		t.Fatal("ConnCounter hid the upstream read waiter fast path")
	}

	reader, countFuncs := N.UnwrapCountReader(wrapped, nil)
	if reader != base {
		t.Fatalf("UnwrapCountReader stopped at %T, want upstream fastPathConn", reader)
	}
	if len(countFuncs) != 1 {
		t.Fatalf("read counter funcs=%d, want 1", len(countFuncs))
	}
	countFuncs[0](123)
	if got := storage.UpCounter.Load(); got != 123 {
		t.Fatalf("read counter recorded %d, want 123", got)
	}
}

func TestConnCounterCountsVectorisedWrites(t *testing.T) {
	base := &fastPathConn{}
	storage := &TrafficStorage{}
	wrapped := NewConnCounter(base, storage)

	vectorised, ok := wrapped.(N.VectorisedWriter)
	if !ok {
		t.Fatalf("ConnCounter does not expose vectorised writes: %T", wrapped)
	}
	if err := vectorised.WriteVectorised([]*buf.Buffer{
		buf.As([]byte("hello")),
		buf.As([]byte("world!")),
	}); err != nil {
		t.Fatal(err)
	}
	if base.vectorisedWrites != 1 {
		t.Fatalf("upstream vectorised writes=%d, want 1", base.vectorisedWrites)
	}
	if base.vectorisedBytes != 11 {
		t.Fatalf("upstream vectorised bytes=%d, want 11", base.vectorisedBytes)
	}
	if got := storage.DownCounter.Load(); got != 11 {
		t.Fatalf("write counter recorded %d, want 11", got)
	}
}

func TestPacketConnCounterUnwrapsCounter(t *testing.T) {
	base := &recordingPacketConn{}
	storage := &TrafficStorage{}
	wrapped := NewPacketConnCounter(base, storage)

	reader, readCountFuncs := N.UnwrapCountPacketReader(wrapped, nil)
	if reader != base {
		t.Fatalf("UnwrapCountPacketReader stopped at %T, want upstream packet conn", reader)
	}
	if len(readCountFuncs) != 1 {
		t.Fatalf("packet read counter funcs=%d, want 1", len(readCountFuncs))
	}
	readCountFuncs[0](321)
	if got := storage.UpCounter.Load(); got != 321 {
		t.Fatalf("packet read counter recorded %d, want 321", got)
	}

	writer, writeCountFuncs := N.UnwrapCountPacketWriter(wrapped, nil)
	if writer != base {
		t.Fatalf("UnwrapCountPacketWriter stopped at %T, want upstream packet conn", writer)
	}
	if len(writeCountFuncs) != 1 {
		t.Fatalf("packet write counter funcs=%d, want 1", len(writeCountFuncs))
	}
	writeCountFuncs[0](654)
	if got := storage.DownCounter.Load(); got != 654 {
		t.Fatalf("packet write counter recorded %d, want 654", got)
	}
}

var _ net.Conn = (*fastPathConn)(nil)
var _ N.ReadWaitCreator = (*fastPathConn)(nil)
var _ N.VectorisedWriter = (*fastPathConn)(nil)
var _ N.PacketConn = (*recordingPacketConn)(nil)
