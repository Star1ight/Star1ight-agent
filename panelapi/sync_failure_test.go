package panelapi

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type syncTestPanel struct {
	users       []User
	fetches     int
	pushes      int
	pushErr     error
	alivePushes int
	aliveErr    error
}

func (p *syncTestPanel) FetchUsers(ctx context.Context) ([]User, error) {
	p.fetches++
	return p.users, nil
}

func (p *syncTestPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	p.pushes++
	return p.pushErr
}

func (p *syncTestPanel) PushAlive(ctx context.Context, alive map[string]map[string][]string) error {
	p.alivePushes++
	return p.aliveErr
}

func TestSyncerDoesNotCommitOnPushFailure(t *testing.T) {
	panel := &syncTestPanel{pushErr: errors.New("temporary panel outage")}
	delta := map[string]map[string][2]int64{
		"vless-tcp": {"1": {123, 456}},
	}
	commits := 0
	s := &Syncer{
		Panel:    panel,
		Snapshot: func() map[string]map[string][2]int64 { return delta },
		Commit: func(got map[string]map[string][2]int64) {
			commits++
		},
	}

	s.flush(context.Background())

	if panel.pushes != 1 {
		t.Fatalf("PushTraffic calls = %d, want 1", panel.pushes)
	}
	if commits != 0 {
		t.Fatalf("Commit called %d times on failed push, want 0", commits)
	}
}

func TestSyncerCommitsOnlyPushedNumericDelta(t *testing.T) {
	panel := &syncTestPanel{}
	delta := map[string]map[string][2]int64{
		"vless-tcp": {"1": {100, 200}},
		"hy2-udp":   {"uuid-user": {300, 400}},
	}
	commits := 0
	var committed map[string]map[string][2]int64
	s := &Syncer{
		Panel:    panel,
		Snapshot: func() map[string]map[string][2]int64 { return delta },
		Commit: func(got map[string]map[string][2]int64) {
			commits++
			committed = got
		},
	}

	s.flush(context.Background())

	if panel.pushes != 1 {
		t.Fatalf("PushTraffic calls = %d, want 1", panel.pushes)
	}
	if commits != 1 {
		t.Fatalf("Commit calls = %d, want 1", commits)
	}
	if _, ok := committed["hy2-udp"]["uuid-user"]; ok {
		t.Fatalf("non-numeric user was committed even though it was not pushed: %#v", committed)
	}
	if committed["vless-tcp"]["1"] != [2]int64{100, 200} {
		t.Fatalf("numeric pushed delta missing from commit: %#v", committed)
	}
}

func TestSyncerDoesNotCommitAliveOnPushFailure(t *testing.T) {
	panel := &syncTestPanel{aliveErr: errors.New("temporary alive outage")}
	alive := map[string]map[string][]string{
		"vless-in": {"1": {"198.51.100.7"}},
	}
	commits := 0
	s := &Syncer{
		Panel: panel,
		Alive: func() map[string]map[string][]string { return alive },
		CommitAlive: func(got map[string]map[string][]string) {
			commits++
		},
	}

	s.flushAlive(context.Background())

	if panel.alivePushes != 1 {
		t.Fatalf("PushAlive calls = %d, want 1", panel.alivePushes)
	}
	if commits != 0 {
		t.Fatalf("CommitAlive called %d times on failed push, want 0", commits)
	}
}

func TestSyncerCommitsAlivePayloadIncludingTombstones(t *testing.T) {
	panel := &syncTestPanel{}
	alive := map[string]map[string][]string{
		"vless-in": {
			"1": {"198.51.100.7"},
			"2": {},
		},
	}
	commits := 0
	var committed map[string]map[string][]string
	s := &Syncer{
		Panel: panel,
		Alive: func() map[string]map[string][]string { return alive },
		CommitAlive: func(got map[string]map[string][]string) {
			commits++
			committed = got
		},
	}

	s.flushAlive(context.Background())

	if panel.alivePushes != 1 {
		t.Fatalf("PushAlive calls = %d, want 1", panel.alivePushes)
	}
	if commits != 1 {
		t.Fatalf("CommitAlive calls = %d, want 1", commits)
	}
	if !reflect.DeepEqual(committed, alive) {
		t.Fatalf("committed alive payload = %#v, want %#v", committed, alive)
	}
}
