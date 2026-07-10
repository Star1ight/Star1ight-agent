package panelapi

import (
	"context"
	"reflect"
	"testing"
)

type recordingPanel struct {
	users      []User
	trafficLog []map[string]map[string][2]int64
	aliveLog   []map[string]map[string][]string
}

func (p *recordingPanel) FetchUsers(ctx context.Context) ([]User, error) {
	return p.users, nil
}

func (p *recordingPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	p.trafficLog = append(p.trafficLog, cloneTrafficDelta(delta))
	return nil
}

func (p *recordingPanel) PushAlive(ctx context.Context, alive map[string]map[string][]string) error {
	p.aliveLog = append(p.aliveLog, cloneAliveDelta(alive))
	return nil
}

func TestParseSourceServerMap(t *testing.T) {
	got, err := ParseSourceServerMap("cnix=51, nbix=52")
	if err != nil {
		t.Fatalf("ParseSourceServerMap: %v", err)
	}
	want := map[string]string{"cnix": "51", "nbix": "52"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("map = %#v, want %#v", got, want)
	}
}

func TestSourceMappedPanelRoutesTaggedTrafficAndStripsSourceSuffix(t *testing.T) {
	base := &recordingPanel{}
	cnix := &recordingPanel{}
	nbix := &recordingPanel{}
	panel := SourceMappedPanel{
		Default: base,
		Routes: map[string]Panel{
			"cnix": cnix,
			"nbix": nbix,
		},
	}

	err := panel.PushTraffic(context.Background(), map[string]map[string][2]int64{
		"ss-in@source=cnix": {"7": {10, 20}},
		"ss-in@source=nbix": {"7": {30, 40}},
		"ss-in":             {"7": {1, 2}},
	})
	if err != nil {
		t.Fatalf("PushTraffic: %v", err)
	}

	assertTrafficLog(t, cnix.trafficLog, []map[string]map[string][2]int64{{"ss-in": {"7": {10, 20}}}})
	assertTrafficLog(t, nbix.trafficLog, []map[string]map[string][2]int64{{"ss-in": {"7": {30, 40}}}})
	assertTrafficLog(t, base.trafficLog, []map[string]map[string][2]int64{{"ss-in": {"7": {1, 2}}}})
}

func TestSourceMappedPanelRoutesTaggedAliveAndStripsSourceSuffix(t *testing.T) {
	base := &recordingPanel{}
	cnix := &recordingPanel{}
	panel := SourceMappedPanel{
		Default: base,
		Routes: map[string]Panel{
			"cnix": cnix,
		},
	}

	err := panel.PushAlive(context.Background(), map[string]map[string][]string{
		"ss-in@source=cnix": {"7": {"103.96.140.122"}},
		"ss-in":             {"7": {}},
	})
	if err != nil {
		t.Fatalf("PushAlive: %v", err)
	}

	assertAliveLog(t, cnix.aliveLog, []map[string]map[string][]string{{"ss-in": {"7": {"103.96.140.122"}}}})
	assertAliveLog(t, base.aliveLog, []map[string]map[string][]string{{"ss-in": {"7": {}}}})
}

func TestSourceFilteredPanelDropsIgnoredSourcesBeforePush(t *testing.T) {
	base := &recordingPanel{}
	panel := SourceFilteredPanel{
		Inner:       base,
		DropSources: map[string]bool{"gomami-backend": true},
	}

	if err := panel.PushTraffic(context.Background(), map[string]map[string][2]int64{
		"ss-in@source=gomami-backend": {"7": {100, 200}},
		"ss-in":                       {"7": {1, 2}},
	}); err != nil {
		t.Fatalf("PushTraffic: %v", err)
	}
	if err := panel.PushAlive(context.Background(), map[string]map[string][]string{
		"ss-in@source=gomami-backend": {"7": {"151.244.134.192"}},
		"ss-in":                       {"7": {"198.51.100.7"}},
	}); err != nil {
		t.Fatalf("PushAlive: %v", err)
	}

	assertTrafficLog(t, base.trafficLog, []map[string]map[string][2]int64{{"ss-in": {"7": {1, 2}}}})
	assertAliveLog(t, base.aliveLog, []map[string]map[string][]string{{"ss-in": {"7": {"198.51.100.7"}}}})
}

func assertTrafficLog(t *testing.T, got, want []map[string]map[string][2]int64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("traffic log = %#v, want %#v", got, want)
	}
}

func assertAliveLog(t *testing.T, got, want []map[string]map[string][]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("alive log = %#v, want %#v", got, want)
	}
}

func cloneTrafficDelta(in map[string]map[string][2]int64) map[string]map[string][2]int64 {
	out := make(map[string]map[string][2]int64, len(in))
	for inbound, users := range in {
		out[inbound] = make(map[string][2]int64, len(users))
		for user, delta := range users {
			out[inbound][user] = delta
		}
	}
	return out
}

func cloneAliveDelta(in map[string]map[string][]string) map[string]map[string][]string {
	out := make(map[string]map[string][]string, len(in))
	for inbound, users := range in {
		out[inbound] = make(map[string][]string, len(users))
		for user, peers := range users {
			out[inbound][user] = copyPeers(peers)
		}
	}
	return out
}
