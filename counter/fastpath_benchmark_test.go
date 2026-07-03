package counter

import (
	"testing"

	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

func BenchmarkConnCounterUnwrapCountReader(b *testing.B) {
	base := &fastPathConn{}
	storage := &TrafficStorage{}
	wrapped := NewConnCounter(base, storage)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader, counters := N.UnwrapCountReader(wrapped, nil)
		if reader != base || len(counters) != 1 {
			b.Fatal("counter unwrap failed")
		}
	}
}

func BenchmarkConnCounterWriteVectorised(b *testing.B) {
	base := &fastPathConn{}
	storage := &TrafficStorage{}
	wrapped := NewConnCounter(base, storage).(N.VectorisedWriter)
	left := []byte("0123456789abcdef")
	right := []byte("fedcba9876543210")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := wrapped.WriteVectorised([]*buf.Buffer{
			buf.As(left),
			buf.As(right),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRateLimiterWaitSmallChunk(b *testing.B) {
	limiter := NewRateLimiter(1 << 30)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := limiter.Wait(1024); err != nil {
			b.Fatal(err)
		}
	}
}
