package main

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"

	N "github.com/sagernet/sing/common/network"
)

type SourceClassifier struct {
	rules []sourceBucketRule
}

type sourceBucketRule struct {
	label    string
	prefixes []netip.Prefix
}

func ParseSourceBuckets(specs []string) (*SourceClassifier, error) {
	classifier := &SourceClassifier{}
	for _, raw := range specs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("source bucket %q must use label=cidr[,cidr...]", raw)
		}
		label := strings.TrimSpace(parts[0])
		if label == "" {
			return nil, fmt.Errorf("source bucket %q is missing a label", raw)
		}
		rule := sourceBucketRule{label: label}
		for _, candidate := range strings.Split(parts[1], ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			prefix, err := parsePrefix(candidate)
			if err != nil {
				return nil, fmt.Errorf("source bucket %q: %w", raw, err)
			}
			rule.prefixes = append(rule.prefixes, prefix.Masked())
		}
		if len(rule.prefixes) == 0 {
			return nil, fmt.Errorf("source bucket %q does not contain any CIDR", raw)
		}
		classifier.rules = append(classifier.rules, rule)
	}
	return classifier, nil
}

func parsePrefix(value string) (netip.Prefix, error) {
	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return netip.Prefix{}, err
		}
		return prefix, nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits), nil
}

func (c *SourceClassifier) Classify(source string) string {
	ip := normalizePeerIP(source)
	if ip == "" {
		return ""
	}
	if c == nil {
		return ip
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	for _, rule := range c.rules {
		for _, prefix := range rule.prefixes {
			if prefix.Contains(addr) {
				return rule.label
			}
		}
	}
	return ip
}

type SessionTracker struct {
	mu         sync.Mutex
	deviceRefs map[string]map[string]map[string]int
	sourceRefs map[string]map[string]map[string]int
	lastAlive  map[string]map[string][]string
	classifier *SourceClassifier
}

func NewSessionTracker(classifier *SourceClassifier) *SessionTracker {
	return &SessionTracker{
		deviceRefs: make(map[string]map[string]map[string]int),
		sourceRefs: make(map[string]map[string]map[string]int),
		lastAlive:  make(map[string]map[string][]string),
		classifier: classifier,
	}
}

func (t *SessionTracker) Open(inbound, user, source string) func() {
	if t == nil || user == "" {
		return func() {}
	}
	inbound = normalizeInboundTag(inbound)
	ip := normalizePeerIP(source)
	label := t.classifier.Classify(source)

	t.mu.Lock()
	if ip != "" {
		inboundDevices := t.deviceRefs[inbound]
		if inboundDevices == nil {
			inboundDevices = make(map[string]map[string]int)
			t.deviceRefs[inbound] = inboundDevices
		}
		userDevices := inboundDevices[user]
		if userDevices == nil {
			userDevices = make(map[string]int)
			inboundDevices[user] = userDevices
		}
		userDevices[ip]++
	}
	if label != "" {
		inboundSources := t.sourceRefs[inbound]
		if inboundSources == nil {
			inboundSources = make(map[string]map[string]int)
			t.sourceRefs[inbound] = inboundSources
		}
		userSources := inboundSources[user]
		if userSources == nil {
			userSources = make(map[string]int)
			inboundSources[user] = userSources
		}
		userSources[label]++
	}
	t.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			t.close(inbound, user, ip, label)
		})
	}
}

func (t *SessionTracker) close(inbound, user, ip, label string) {
	if t == nil || user == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if ip != "" {
		if inboundDevices := t.deviceRefs[inbound]; inboundDevices != nil {
			if userDevices := inboundDevices[user]; userDevices != nil {
				userDevices[ip]--
				if userDevices[ip] <= 0 {
					delete(userDevices, ip)
				}
				if len(userDevices) == 0 {
					delete(inboundDevices, user)
				}
			}
			if len(inboundDevices) == 0 {
				delete(t.deviceRefs, inbound)
			}
		}
	}

	if label != "" {
		if inboundSources := t.sourceRefs[inbound]; inboundSources != nil {
			if userSources := inboundSources[user]; userSources != nil {
				userSources[label]--
				if userSources[label] <= 0 {
					delete(userSources, label)
				}
				if len(userSources) == 0 {
					delete(inboundSources, user)
				}
			}
			if len(inboundSources) == 0 {
				delete(t.sourceRefs, inbound)
			}
		}
	}
}

func (t *SessionTracker) DevicesSnapshot() map[string]map[string][]string {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make(map[string]map[string][]string, len(t.deviceRefs))
	for inbound, users := range t.deviceRefs {
		out[inbound] = make(map[string][]string, len(users))
		for user, peers := range users {
			ips := make([]string, 0, len(peers))
			for ip := range peers {
				ips = append(ips, ip)
			}
			sort.Strings(ips)
			out[inbound][user] = ips
		}
	}
	return out
}

func (t *SessionTracker) AliveDelta() map[string]map[string][]string {
	if t == nil {
		return nil
	}
	current := t.DevicesSnapshot()

	t.mu.Lock()
	defer t.mu.Unlock()

	out := make(map[string]map[string][]string)
	for inbound, users := range current {
		for user, ips := range users {
			if !sameStringSlice(t.lastAlive[inbound][user], ips) {
				if out[inbound] == nil {
					out[inbound] = make(map[string][]string)
				}
				out[inbound][user] = append([]string(nil), ips...)
			}
		}
	}
	for inbound, users := range t.lastAlive {
		currentUsers := current[inbound]
		for user := range users {
			if currentUsers == nil {
				if out[inbound] == nil {
					out[inbound] = make(map[string][]string)
				}
				out[inbound][user] = []string{}
				continue
			}
			if _, ok := currentUsers[user]; !ok {
				if out[inbound] == nil {
					out[inbound] = make(map[string][]string)
				}
				out[inbound][user] = []string{}
			}
		}
	}
	return out
}

func (t *SessionTracker) CommitAlive(payload map[string]map[string][]string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	for inbound, users := range payload {
		if len(users) == 0 {
			delete(t.lastAlive, inbound)
			continue
		}
		if t.lastAlive[inbound] == nil {
			t.lastAlive[inbound] = make(map[string][]string)
		}
		for user, ips := range users {
			if len(ips) == 0 {
				delete(t.lastAlive[inbound], user)
				continue
			}
			copied := append([]string(nil), ips...)
			sort.Strings(copied)
			t.lastAlive[inbound][user] = copied
		}
		if len(t.lastAlive[inbound]) == 0 {
			delete(t.lastAlive, inbound)
		}
	}
}

func (t *SessionTracker) SourceSnapshot() map[string]map[string]map[string]int {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make(map[string]map[string]map[string]int, len(t.sourceRefs))
	for inbound, users := range t.sourceRefs {
		out[inbound] = make(map[string]map[string]int, len(users))
		for user, labels := range users {
			out[inbound][user] = make(map[string]int, len(labels))
			for label, count := range labels {
				out[inbound][user][label] = count
			}
		}
	}
	return out
}

func normalizeInboundTag(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return value
}

func normalizePeerIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.String()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr.String()
		}
		return host
	}
	return value
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type trackedConn struct {
	net.Conn
	release func()
}

func (c *trackedConn) Close() error {
	defer c.release()
	return c.Conn.Close()
}

type trackedPacketConn struct {
	N.PacketConn
	release func()
}

func (c *trackedPacketConn) Close() error {
	defer c.release()
	return c.PacketConn.Close()
}

func (c *trackedPacketConn) Upstream() any {
	return c.PacketConn
}
