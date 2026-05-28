package counter

import (
	"testing"
	"time"
)

func TestRateLimiterAllowRecoversAfterClockMovesBackward(t *testing.T) {
	limiter := NewRateLimiter(1)
	limiter.mu.Lock()
	limiter.last = time.Now().Add(time.Hour)
	limiter.tokens = 0
	limiter.mu.Unlock()

	ok, err := limiter.Allow(1)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected first packet after clock rollback to be over limit")
	}

	limiter.mu.Lock()
	last := limiter.last
	limiter.mu.Unlock()
	if time.Until(last) > time.Minute {
		t.Fatalf("limiter did not reset future timestamp after clock rollback: last=%s", last)
	}
}
