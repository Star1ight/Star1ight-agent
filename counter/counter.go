package counter

import (
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type TrafficCounter struct {
	Counters sync.Map // map[string]*TrafficStorage, key is stable panel user id/name
}

type TrafficStorage struct {
	UpCounter     atomic.Int64
	DownCounter   atomic.Int64
	CommittedUp   atomic.Int64
	CommittedDown atomic.Int64
}

func NewTrafficCounter() *TrafficCounter { return &TrafficCounter{} }

func (c *TrafficCounter) GetCounter(user string) *TrafficStorage {
	if v, ok := c.Counters.Load(user); ok {
		return v.(*TrafficStorage)
	}
	s := &TrafficStorage{}
	v, _ := c.Counters.LoadOrStore(user, s)
	return v.(*TrafficStorage)
}

func (c *TrafficCounter) Snapshot(reset bool) map[string][2]int64 {
	out := map[string][2]int64{}
	c.Counters.Range(func(k, v any) bool {
		s := v.(*TrafficStorage)
		out[k.(string)] = [2]int64{s.UpCounter.Load(), s.DownCounter.Load()}
		return true
	})
	return out
}

func (c *TrafficCounter) SnapshotDelta() map[string][2]int64 {
	out := map[string][2]int64{}
	c.Counters.Range(func(k, v any) bool {
		s := v.(*TrafficStorage)
		up := s.UpCounter.Load() - s.CommittedUp.Load()
		down := s.DownCounter.Load() - s.CommittedDown.Load()
		if up > 0 || down > 0 {
			out[k.(string)] = [2]int64{up, down}
		}
		return true
	})
	return out
}

// CommitSnapshot must be called serially and exactly once for a successfully pushed snapshot.
// New traffic arriving between SnapshotDelta and CommitSnapshot is intentionally left for the next snapshot.
func (c *TrafficCounter) CommitSnapshot(snapshot map[string][2]int64) {
	for user, d := range snapshot {
		if v, ok := c.Counters.Load(user); ok {
			s := v.(*TrafficStorage)
			s.CommittedUp.Add(d[0])
			s.CommittedDown.Add(d[1])
		}
	}
}

func (c *TrafficCounter) RemoveAbsent(active map[string]struct{}) {
	c.Counters.Range(func(k, v any) bool {
		user := k.(string)
		if _, ok := active[user]; !ok {
			s := v.(*TrafficStorage)
			if s.UpCounter.Load() == s.CommittedUp.Load() && s.DownCounter.Load() == s.CommittedDown.Load() {
				c.Counters.Delete(user)
			}
		}
		return true
	})
}

type ConnCounter struct {
	N.ExtendedConn
	storage  *TrafficStorage
	activity func(int64)
}

func NewConnCounter(conn net.Conn, s *TrafficStorage) net.Conn {
	return NewConnCounterWithActivity(conn, s, nil)
}

func NewConnCounterWithActivity(conn net.Conn, s *TrafficStorage, activity func(int64)) net.Conn {
	return &ConnCounter{ExtendedConn: bufio.NewExtendedConn(conn), storage: s, activity: activity}
}

func (c *ConnCounter) countUp(n int64) {
	if n <= 0 {
		return
	}
	if c.activity != nil {
		c.activity(n)
	}
	c.storage.UpCounter.Add(n)
}

func (c *ConnCounter) countDown(n int64) {
	if n <= 0 {
		return
	}
	if c.activity != nil {
		c.activity(n)
	}
	c.storage.DownCounter.Add(n)
}

func (c *ConnCounter) Read(b []byte) (int, error) {
	n, err := c.ExtendedConn.Read(b)
	if n > 0 {
		c.countUp(int64(n))
	}
	return n, err
}
func (c *ConnCounter) Write(b []byte) (int, error) {
	n, err := c.ExtendedConn.Write(b)
	if n > 0 {
		c.countDown(int64(n))
	}
	return n, err
}
func (c *ConnCounter) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err == nil && buffer.Len() > 0 {
		c.countUp(int64(buffer.Len()))
	}
	return err
}
func (c *ConnCounter) WriteBuffer(buffer *buf.Buffer) error {
	n := buffer.Len()
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err == nil && n > 0 {
		c.countDown(int64(n))
	}
	return err
}

func (c *ConnCounter) CreateReadWaiter() (N.ReadWaiter, bool) {
	readWaiter, ok := bufio.CreateReadWaiter(N.UnwrapReader(c.ExtendedConn))
	if !ok {
		return nil, false
	}
	return &countingReadWaiter{ReadWaiter: readWaiter, count: c.countUp}, true
}

func (c *ConnCounter) CreateVectorisedReadWaiter() (N.VectorisedReadWaiter, bool) {
	readWaiter, ok := bufio.CreateVectorisedReadWaiter(N.UnwrapReader(c.ExtendedConn))
	if !ok {
		return nil, false
	}
	return &countingVectorisedReadWaiter{VectorisedReadWaiter: readWaiter, count: c.countUp}, true
}

func (c *ConnCounter) WriteVectorised(buffers []*buf.Buffer) error {
	dataLen := buf.LenMulti(buffers)
	writer := bufio.NewVectorisedWriter(N.UnwrapWriter(c.ExtendedConn))
	err := writer.WriteVectorised(buffers)
	if err == nil && dataLen > 0 {
		c.countDown(int64(dataLen))
	}
	return err
}

func (c *ConnCounter) ReadFrom(reader io.Reader) (int64, error) {
	n, err := io.Copy(c.ExtendedConn, reader)
	if n > 0 {
		c.countDown(n)
	}
	return n, err
}

func (c *ConnCounter) WriteTo(writer io.Writer) (int64, error) {
	n, err := io.Copy(writer, c.ExtendedConn)
	if n > 0 {
		c.countUp(n)
	}
	return n, err
}

func (c *ConnCounter) UnwrapReader() (io.Reader, []N.CountFunc) {
	return N.UnwrapReader(c.ExtendedConn), []N.CountFunc{c.countUp}
}

func (c *ConnCounter) UnwrapWriter() (io.Writer, []N.CountFunc) {
	return N.UnwrapWriter(c.ExtendedConn), []N.CountFunc{c.countDown}
}

func (c *ConnCounter) UpstreamReader() any { return c.ExtendedConn }
func (c *ConnCounter) UpstreamWriter() any { return c.ExtendedConn }
func (c *ConnCounter) Upstream() any       { return c.ExtendedConn }

type PacketConnCounter struct {
	N.PacketConn
	storage  *TrafficStorage
	activity func(int64)
}

func NewPacketConnCounter(conn N.PacketConn, s *TrafficStorage) N.PacketConn {
	return NewPacketConnCounterWithActivity(conn, s, nil)
}

func NewPacketConnCounterWithActivity(conn N.PacketConn, s *TrafficStorage, activity func(int64)) N.PacketConn {
	return &PacketConnCounter{PacketConn: conn, storage: s, activity: activity}
}

func (p *PacketConnCounter) countUp(n int64) {
	if n <= 0 {
		return
	}
	if p.activity != nil {
		p.activity(n)
	}
	p.storage.UpCounter.Add(n)
}

func (p *PacketConnCounter) countDown(n int64) {
	if n <= 0 {
		return
	}
	if p.activity != nil {
		p.activity(n)
	}
	p.storage.DownCounter.Add(n)
}

func (p *PacketConnCounter) ReadPacket(buff *buf.Buffer) (M.Socksaddr, error) {
	dest, err := p.PacketConn.ReadPacket(buff)
	if err == nil && buff.Len() > 0 {
		p.countUp(int64(buff.Len()))
	}
	return dest, err
}
func (p *PacketConnCounter) WritePacket(buff *buf.Buffer, dest M.Socksaddr) error {
	n := buff.Len()
	err := p.PacketConn.WritePacket(buff, dest)
	if err == nil && n > 0 {
		p.countDown(int64(n))
	}
	return err
}

func (p *PacketConnCounter) CreateReadWaiter() (N.PacketReadWaiter, bool) {
	readWaiter, ok := bufio.CreatePacketReadWaiter(N.UnwrapPacketReader(p.PacketConn))
	if !ok {
		return nil, false
	}
	return &countingPacketReadWaiter{PacketReadWaiter: readWaiter, count: p.countUp}, true
}

func (p *PacketConnCounter) UnwrapPacketReader() (N.PacketReader, []N.CountFunc) {
	return N.UnwrapPacketReader(p.PacketConn), []N.CountFunc{p.countUp}
}

func (p *PacketConnCounter) UnwrapPacketWriter() (N.PacketWriter, []N.CountFunc) {
	return N.UnwrapPacketWriter(p.PacketConn), []N.CountFunc{p.countDown}
}

func (p *PacketConnCounter) UpstreamReader() any { return p.PacketConn }
func (p *PacketConnCounter) UpstreamWriter() any { return p.PacketConn }
func (p *PacketConnCounter) Upstream() any       { return p.PacketConn }

type countingReadWaiter struct {
	N.ReadWaiter
	count N.CountFunc
}

func (w *countingReadWaiter) WaitReadBuffer() (*buf.Buffer, error) {
	buffer, err := w.ReadWaiter.WaitReadBuffer()
	if err == nil && buffer != nil && buffer.Len() > 0 {
		w.count(int64(buffer.Len()))
	}
	return buffer, err
}

type countingVectorisedReadWaiter struct {
	N.VectorisedReadWaiter
	count N.CountFunc
}

func (w *countingVectorisedReadWaiter) WaitReadBuffers() ([]*buf.Buffer, error) {
	buffers, err := w.VectorisedReadWaiter.WaitReadBuffers()
	if err == nil {
		if dataLen := buf.LenMulti(buffers); dataLen > 0 {
			w.count(int64(dataLen))
		}
	}
	return buffers, err
}

type countingPacketReadWaiter struct {
	N.PacketReadWaiter
	count N.CountFunc
}

func (w *countingPacketReadWaiter) WaitReadPacket() (*buf.Buffer, M.Socksaddr, error) {
	buffer, destination, err := w.PacketReadWaiter.WaitReadPacket()
	if err == nil && buffer != nil && buffer.Len() > 0 {
		w.count(int64(buffer.Len()))
	}
	return buffer, destination, err
}
