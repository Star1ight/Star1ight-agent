package panelapi

import (
	"context"
	"fmt"
	"strings"
)

const sourceTagMarker = "@source="

// ParseSourceServerMap parses label=node_id pairs used to split one data-plane
// listener across multiple XBoard server records.
func ParseSourceServerMap(spec string) (map[string]string, error) {
	out := make(map[string]string)
	for _, raw := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ';' }) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("source server map %q must use label=node_id", raw)
		}
		label := strings.TrimSpace(parts[0])
		nodeID := strings.TrimSpace(parts[1])
		if label == "" || nodeID == "" {
			return nil, fmt.Errorf("source server map %q must include label and node_id", raw)
		}
		out[label] = nodeID
	}
	return out, nil
}

type SourceMappedPanel struct {
	Default Panel
	Routes  map[string]Panel
}

func (m SourceMappedPanel) FetchUsers(ctx context.Context) ([]User, error) {
	var merged []User
	seen := map[int]bool{}
	add := func(panel Panel) error {
		if panel == nil {
			return nil
		}
		users, err := panel.FetchUsers(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			if u.ID > 0 {
				if seen[u.ID] {
					continue
				}
				seen[u.ID] = true
			}
			merged = append(merged, u)
		}
		return nil
	}
	if err := add(m.Default); err != nil {
		return nil, err
	}
	for _, panel := range m.Routes {
		if err := add(panel); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

func (m SourceMappedPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	for panel, routed := range m.routeTraffic(delta) {
		if len(routed) == 0 || panel == nil {
			continue
		}
		if err := panel.PushTraffic(ctx, routed); err != nil {
			return fmt.Errorf("push source-mapped traffic: %w", err)
		}
	}
	return nil
}

func (m SourceMappedPanel) PushAlive(ctx context.Context, alive map[string]map[string][]string) error {
	for panel, routed := range m.routeAlive(alive) {
		if len(routed) == 0 || panel == nil {
			continue
		}
		if err := panel.PushAlive(ctx, routed); err != nil {
			return fmt.Errorf("push source-mapped alive: %w", err)
		}
	}
	return nil
}

func (m SourceMappedPanel) routeTraffic(delta map[string]map[string][2]int64) map[Panel]map[string]map[string][2]int64 {
	out := make(map[Panel]map[string]map[string][2]int64)
	for inbound, users := range delta {
		panel, baseInbound := m.routeForInbound(inbound)
		if panel == nil {
			continue
		}
		if out[panel] == nil {
			out[panel] = make(map[string]map[string][2]int64)
		}
		if out[panel][baseInbound] == nil {
			out[panel][baseInbound] = make(map[string][2]int64)
		}
		for user, d := range users {
			old := out[panel][baseInbound][user]
			old[0] += d[0]
			old[1] += d[1]
			out[panel][baseInbound][user] = old
		}
	}
	return out
}

func (m SourceMappedPanel) routeAlive(alive map[string]map[string][]string) map[Panel]map[string]map[string][]string {
	out := make(map[Panel]map[string]map[string][]string)
	for inbound, users := range alive {
		panel, baseInbound := m.routeForInbound(inbound)
		if panel == nil {
			continue
		}
		if out[panel] == nil {
			out[panel] = make(map[string]map[string][]string)
		}
		if out[panel][baseInbound] == nil {
			out[panel][baseInbound] = make(map[string][]string)
		}
		for user, peers := range users {
			out[panel][baseInbound][user] = copyPeers(peers)
		}
	}
	return out
}

func (m SourceMappedPanel) routeForInbound(inbound string) (Panel, string) {
	baseInbound, source := splitSourceInbound(inbound)
	if source != "" {
		if panel := m.Routes[source]; panel != nil {
			return panel, baseInbound
		}
	}
	return m.Default, baseInbound
}

func splitSourceInbound(inbound string) (baseInbound, source string) {
	baseInbound = inbound
	if idx := strings.LastIndex(inbound, sourceTagMarker); idx >= 0 {
		baseInbound = inbound[:idx]
		source = inbound[idx+len(sourceTagMarker):]
	}
	if strings.TrimSpace(baseInbound) == "" {
		baseInbound = "default"
	}
	return baseInbound, source
}

func copyPeers(peers []string) []string {
	if len(peers) == 0 {
		return []string{}
	}
	return append([]string(nil), peers...)
}
