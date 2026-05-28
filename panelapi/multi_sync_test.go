package panelapi

import (
	"context"
	"testing"
)

type recordingPanel struct {
	users  []User
	pushes []map[string][2]int64
}

func (p *recordingPanel) FetchUsers(ctx context.Context) ([]User, error) {
	return p.users, nil
}

func (p *recordingPanel) PushTraffic(ctx context.Context, delta map[string][2]int64) error {
	copyDelta := make(map[string][2]int64, len(delta))
	for user, v := range delta {
		copyDelta[user] = v
	}
	p.pushes = append(p.pushes, copyDelta)
	return nil
}

func TestMultiSyncerRoutesTrafficByInboundTag(t *testing.T) {
	vlessPanel := &recordingPanel{}
	hy2Panel := &recordingPanel{}
	delta := map[string]map[string][2]int64{
		"vless-tcp": {
			"1": {100, 200},
		},
		"hy2-udp": {
			"1":         {300, 400},
			"uuid-user": {500, 600},
		},
	}
	commits := 0
	var committed map[string]map[string][2]int64

	s := &MultiSyncer{
		Snapshot: func() map[string]map[string][2]int64 { return delta },
		Commit: func(got map[string]map[string][2]int64) {
			commits++
			committed = got
		},
		Routes: []PanelRoute{
			{Panel: vlessPanel, InboundTags: []string{"vless-tcp"}, FetchUsers: true},
			{Panel: hy2Panel, InboundTags: []string{"hy2-udp"}},
		},
	}

	s.flush(context.Background())

	if len(vlessPanel.pushes) != 1 {
		t.Fatalf("vless pushes = %d, want 1", len(vlessPanel.pushes))
	}
	if got := vlessPanel.pushes[0]["1"]; got != [2]int64{100, 200} {
		t.Fatalf("vless pushed %#v, want user 1 [100 200]", vlessPanel.pushes[0])
	}
	if len(hy2Panel.pushes) != 1 {
		t.Fatalf("hy2 pushes = %d, want 1", len(hy2Panel.pushes))
	}
	if got := hy2Panel.pushes[0]["1"]; got != [2]int64{300, 400} {
		t.Fatalf("hy2 pushed %#v, want user 1 [300 400]", hy2Panel.pushes[0])
	}
	if _, ok := hy2Panel.pushes[0]["uuid-user"]; ok {
		t.Fatalf("hy2 pushed non-numeric user: %#v", hy2Panel.pushes[0])
	}
	if commits != 1 {
		t.Fatalf("commits = %d, want 1", commits)
	}
	if got := committed["vless-tcp"]["1"]; got != [2]int64{100, 200} {
		t.Fatalf("committed vless delta = %#v", committed)
	}
	if got := committed["hy2-udp"]["1"]; got != [2]int64{300, 400} {
		t.Fatalf("committed hy2 delta = %#v", committed)
	}
	if _, ok := committed["hy2-udp"]["uuid-user"]; ok {
		t.Fatalf("committed unpushed non-numeric delta: %#v", committed)
	}
}

func TestMultiSyncerFetchesUsersFromDesignatedRouteOnly(t *testing.T) {
	vlessPanel := &recordingPanel{users: []User{{ID: 1, UUID: "uuid-1"}}}
	hy2Panel := &recordingPanel{users: []User{{ID: 2, UUID: "uuid-2"}}}
	var applied []User

	s := &MultiSyncer{
		Snapshot: func() map[string]map[string][2]int64 { return nil },
		Commit:   func(map[string]map[string][2]int64) {},
		Users: func(users []User) error {
			applied = append(applied, users...)
			return nil
		},
		Routes: []PanelRoute{
			{Panel: vlessPanel, InboundTags: []string{"vless-tcp"}, FetchUsers: true},
			{Panel: hy2Panel, InboundTags: []string{"hy2-udp"}},
		},
	}

	s.syncOnce(context.Background())

	if len(applied) != 1 || applied[0].ID != 1 {
		t.Fatalf("applied users = %#v, want only vless route users", applied)
	}
}
