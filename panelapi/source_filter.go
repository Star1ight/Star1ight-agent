package panelapi

import "context"

// SourceFilteredPanel drops selected source-labelled inbounds before they reach
// the panel. It is used for backend legs of two-hop nodes where a frontend agent
// already accounts the user traffic and online device.
type SourceFilteredPanel struct {
	Inner       Panel
	DropSources map[string]bool
}

func (p SourceFilteredPanel) FetchUsers(ctx context.Context) ([]User, error) {
	if p.Inner == nil {
		return nil, nil
	}
	return p.Inner.FetchUsers(ctx)
}

func (p SourceFilteredPanel) PushTraffic(ctx context.Context, delta map[string]map[string][2]int64) error {
	if p.Inner == nil {
		return nil
	}
	filtered := p.filterTraffic(delta)
	if len(filtered) == 0 {
		return nil
	}
	return p.Inner.PushTraffic(ctx, filtered)
}

func (p SourceFilteredPanel) PushAlive(ctx context.Context, alive map[string]map[string][]string) error {
	if p.Inner == nil {
		return nil
	}
	filtered := p.filterAlive(alive)
	if len(filtered) == 0 {
		return nil
	}
	return p.Inner.PushAlive(ctx, filtered)
}

func (p SourceFilteredPanel) filterTraffic(delta map[string]map[string][2]int64) map[string]map[string][2]int64 {
	out := make(map[string]map[string][2]int64, len(delta))
	for inbound, users := range delta {
		if p.shouldDrop(inbound) {
			continue
		}
		out[inbound] = make(map[string][2]int64, len(users))
		for user, d := range users {
			out[inbound][user] = d
		}
	}
	return out
}

func (p SourceFilteredPanel) filterAlive(alive map[string]map[string][]string) map[string]map[string][]string {
	out := make(map[string]map[string][]string, len(alive))
	for inbound, users := range alive {
		if p.shouldDrop(inbound) {
			continue
		}
		out[inbound] = make(map[string][]string, len(users))
		for user, peers := range users {
			out[inbound][user] = copyPeers(peers)
		}
	}
	return out
}

func (p SourceFilteredPanel) shouldDrop(inbound string) bool {
	if len(p.DropSources) == 0 {
		return false
	}
	_, source := splitSourceInbound(inbound)
	return source != "" && p.DropSources[source]
}
