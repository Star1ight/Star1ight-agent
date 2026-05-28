package main

import (
	"fmt"
	"strings"

	"mini-sb-agent/panelapi"
)

type legacyPanelRoute struct {
	nodeType string
	nodeID   string
}

type panelRouteConfig struct {
	nodeType    string
	inboundTag  string
	nodeID      string
	inboundTags []string
	panel       panelapi.Panel
}

func panelRoutesForSyncer(routes []panelRouteConfig) []panelapi.PanelRoute {
	out := make([]panelapi.PanelRoute, 0, len(routes))
	for i, route := range routes {
		out = append(out, panelapi.PanelRoute{
			Panel:       route.panel,
			InboundTags: route.inboundTags,
			FetchUsers:  i == 0,
		})
	}
	return out
}

func parsePanelRoutes(raw []string, panelURL, panelToken string, legacy ...legacyPanelRoute) ([]panelRouteConfig, error) {
	if len(raw) == 0 {
		if len(legacy) == 0 || legacy[0].nodeID == "" || panelURL == "" {
			return nil, nil
		}
		l := legacy[0]
		return []panelRouteConfig{{
			nodeType: l.nodeType,
			nodeID:   l.nodeID,
			panel:    panelapi.NewClient(panelURL, panelToken, l.nodeID, l.nodeType),
		}}, nil
	}

	routes := make([]panelRouteConfig, 0, len(raw))
	for _, item := range raw {
		parts := strings.Split(item, ":")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid panel route %q, want node_type:inbound_tag:node_id", item)
		}
		routes = append(routes, panelRouteConfig{
			nodeType:    parts[0],
			inboundTag:  parts[1],
			nodeID:      parts[2],
			inboundTags: []string{parts[1]},
			panel:       panelapi.NewClient(panelURL, panelToken, parts[2], parts[0]),
		})
	}
	return routes, nil
}
