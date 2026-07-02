package panelapi

import (
	"context"
	"errors"
	"testing"
)

type fakePanel struct {
	users     []User
	err       error
	push      int
	alivePush int
}

func (f *fakePanel) FetchUsers(ctx context.Context) ([]User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.users, nil
}

func (f *fakePanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	f.push++
	return f.err
}

func (f *fakePanel) PushAlive(ctx context.Context, alive map[string]map[string][]string) error {
	f.alivePush++
	return f.err
}

func TestMultiPanelMergesUsersAndDeduplicatesByID(t *testing.T) {
	panel := MultiPanel{Panels: []Panel{
		&fakePanel{users: []User{{ID: 1, UUID: "a"}, {ID: 2, UUID: "b"}}},
		&fakePanel{users: []User{{ID: 2, UUID: "b2"}, {ID: 3, UUID: "c"}}},
	}}
	users, err := panel.FetchUsers(context.Background())
	if err != nil {
		t.Fatalf("FetchUsers: %v", err)
	}
	if len(users) != 3 || users[0].ID != 1 || users[1].ID != 2 || users[2].ID != 3 {
		t.Fatalf("users = %+v", users)
	}
}

func TestMultiPanelPushesTrafficToAllPanels(t *testing.T) {
	a := &fakePanel{}
	b := &fakePanel{}
	panel := MultiPanel{Panels: []Panel{a, b}}
	if err := panel.PushTraffic(context.Background(), map[string]map[string][2]int64{"vless-in": {"1": {1, 2}}}); err != nil {
		t.Fatalf("PushTraffic: %v", err)
	}
	if a.push != 1 || b.push != 1 {
		t.Fatalf("push counts = %d/%d", a.push, b.push)
	}
}

func TestMultiPanelPushesAliveToAllPanels(t *testing.T) {
	a := &fakePanel{}
	b := &fakePanel{}
	panel := MultiPanel{Panels: []Panel{a, b}}
	if err := panel.PushAlive(context.Background(), map[string]map[string][]string{"vless-in": {"1": {"198.51.100.7"}}}); err != nil {
		t.Fatalf("PushAlive: %v", err)
	}
	if a.alivePush != 1 || b.alivePush != 1 {
		t.Fatalf("alive push counts = %d/%d", a.alivePush, b.alivePush)
	}
}

func TestMultiPanelStopsOnError(t *testing.T) {
	want := errors.New("boom")
	panel := MultiPanel{Panels: []Panel{&fakePanel{err: want}}}
	if _, err := panel.FetchUsers(context.Background()); err == nil {
		t.Fatal("FetchUsers error = nil")
	}
}
