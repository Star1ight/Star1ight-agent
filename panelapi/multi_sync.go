package panelapi

import (
	"context"
	"log"
	"time"
)

// PanelRoute binds one panel node to one or more inbound tags. It lets a
// single process run multiple protocol inbounds while still pushing each
// protocol's traffic to the panel node that owns it.
type PanelRoute struct {
	Panel       Panel
	InboundTags []string
	FetchUsers  bool
}

type MultiSyncer struct {
	Snapshot func() map[string]map[string][2]int64
	Commit   func(map[string]map[string][2]int64)
	Users    func([]User) error
	Every    time.Duration
	Routes   []PanelRoute
}

func (s *MultiSyncer) Run(ctx context.Context) {
	if s.Every <= 0 {
		s.Every = time.Minute
	}
	s.syncOnce(ctx)
	ticker := time.NewTicker(s.Every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s.flush(flushCtx)
			cancel()
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *MultiSyncer) syncOnce(ctx context.Context) {
	for _, route := range s.Routes {
		if route.Panel == nil || !route.FetchUsers {
			continue
		}
		users, err := route.Panel.FetchUsers(ctx)
		if err != nil {
			log.Printf("panel api fetch users: %v", err)
			continue
		}
		if s.Users != nil {
			if err := s.Users(users); err != nil {
				log.Printf("apply users: %v", err)
			}
		}
	}
	s.flush(ctx)
}

func (s *MultiSyncer) flush(ctx context.Context) {
	if s.Snapshot == nil || s.Commit == nil {
		return
	}
	delta := s.Snapshot()
	if len(delta) == 0 {
		return
	}
	committed := make(map[string]map[string][2]int64)
	for _, route := range s.Routes {
		if route.Panel == nil {
			continue
		}
		routed := filterDeltaByInboundTags(delta, route.InboundTags)
		flat := flatten(routed)
		if len(flat) == 0 {
			continue
		}
		if err := route.Panel.PushTraffic(ctx, flat); err != nil {
			log.Printf("panel api push traffic: %v", err)
			continue
		}
		mergeCommitDelta(committed, commitDelta(routed))
	}
	if len(committed) > 0 {
		s.Commit(committed)
	}
}

func filterDeltaByInboundTags(delta map[string]map[string][2]int64, tags []string) map[string]map[string][2]int64 {
	if len(tags) == 0 {
		return delta
	}
	out := make(map[string]map[string][2]int64)
	for _, tag := range tags {
		if users, ok := delta[tag]; ok {
			out[tag] = users
		}
	}
	return out
}

func mergeCommitDelta(dst, src map[string]map[string][2]int64) {
	for inbound, users := range src {
		if dst[inbound] == nil {
			dst[inbound] = make(map[string][2]int64)
		}
		for user, v := range users {
			dst[inbound][user] = v
		}
	}
}
