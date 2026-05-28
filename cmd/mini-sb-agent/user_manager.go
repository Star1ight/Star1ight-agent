package main

import (
	"fmt"
	"sync"

	"mini-sb-agent/counter"
	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/vless"
)

// limiterPair holds separate read (rx) and write (tx) rate limiters so that
// upstream and downstream traffic do not compete for the same token bucket.
// This prevents ACK / control-packet starvation: a saturated download no
// longer blocks the upload direction (and vice versa).
type limiterPair struct {
	rx *counter.RateLimiter // applied on the Read / ReadPacket path
	tx *counter.RateLimiter // applied on the Write / WritePacket path
}

func newLimiterPair(bytesPerSecond int64) limiterPair {
	return limiterPair{
		rx: counter.NewRateLimiter(bytesPerSecond),
		tx: counter.NewRateLimiter(bytesPerSecond),
	}
}

func (p limiterPair) SetRate(bytesPerSecond int64) {
	p.rx.SetRate(bytesPerSecond)
	p.tx.SetRate(bytesPerSecond)
}

func (p limiterPair) Close() {
	p.rx.Close()
	p.tx.Close()
}

type UserManager struct {
	mu        sync.Mutex
	users     map[int]panelapi.User
	bySecret  map[string]int
	nodeLimit *limiterPair
	limiters  map[int]*limiterPair
}

func NewUserManager(nodeMbps int) *UserManager {
	var nodeLim *limiterPair
	if nodeMbps > 0 {
		p := newLimiterPair(mbpsToBytes(nodeMbps))
		nodeLim = &p
	}
	return &UserManager{
		users:     make(map[int]panelapi.User),
		bySecret:  make(map[string]int),
		nodeLimit: nodeLim,
		limiters:  make(map[int]*limiterPair),
	}
}

func mbpsToBytes(mbps int) int64 {
	if mbps <= 0 {
		return 0
	}
	return int64(mbps) * 1000 * 1000 / 8
}

// sameUser reports whether all panel user fields are unchanged.
func sameUser(a, b panelapi.User) bool {
	return a.ID == b.ID && a.UUID == b.UUID && a.Password == b.Password && a.Name == b.Name && a.SpeedLimit == b.SpeedLimit
}

// sameCredentials reports whether a and b have the same inbound credentials.
// SpeedLimit is intentionally excluded: speed-limit-only changes must update
// the limiter without re-adding the same VLESS/HY2 user to sing-box.
func sameCredentials(a, b panelapi.User) bool {
	return a.ID == b.ID && a.UUID == b.UUID && a.Password == b.Password && a.Name == b.Name
}

type userDiff struct {
	addVless  []option.VLESSUser
	delVless  []string
	addHy2    []option.Hysteria2User
	addHy2IDs []int
	delHy2    []string
}

func vlessUserFromPanelUser(u panelapi.User) option.VLESSUser {
	return option.VLESSUser{Name: u.UUID, UUID: u.UUID, Flow: "xtls-rprx-vision"}
}

func (m *UserManager) ApplyBox(inbounds map[string]adapter.Inbound, users []panelapi.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := make(map[int]panelapi.User, len(users))
	nextSecrets := make(map[string]int, len(users)*3)
	for _, u := range users {
		if u.ID <= 0 {
			continue
		}
		next[u.ID] = u
		for _, secret := range []string{u.UUID, u.Password, u.Name} {
			if secret != "" {
				nextSecrets[secret] = u.ID
			}
		}
	}

	diff := m.diffUsersLocked(next)

	for tag, raw := range inbounds {
		switch in := raw.(type) {
		case *vless.Inbound:
			if len(diff.delVless) > 0 {
				if err := in.DelUsers(diff.delVless); err != nil {
					return fmt.Errorf("delete vless users from %s: %w", tag, err)
				}
			}
			if len(diff.addVless) > 0 {
				if err := in.AddUsers(diff.addVless); err != nil {
					return fmt.Errorf("add vless users to %s: %w", tag, err)
				}
			}
		case *hysteria2.Inbound:
			if len(diff.delHy2) > 0 {
				if err := in.DelUsers(diff.delHy2); err != nil {
					return fmt.Errorf("delete hysteria2 users from %s: %w", tag, err)
				}
			}
			if len(diff.addHy2) > 0 {
				if err := in.AddUsers(diff.addHy2, diff.addHy2IDs); err != nil {
					return fmt.Errorf("add hysteria2 users to %s: %w", tag, err)
				}
			}
		}
	}

	m.users = next
	m.bySecret = nextSecrets
	return nil
}

// diffUsersLocked computes inbound add/delete operations and updates limiter
// objects. It must be called with m.mu held.
func (m *UserManager) diffUsersLocked(next map[int]panelapi.User) userDiff {
	var diff userDiff

	for id, old := range m.users {
		nu, ok := next[id]
		if !ok {
			if old.UUID != "" {
				diff.delVless = append(diff.delVless, old.UUID)
			}
			if old.Password != "" {
				diff.delHy2 = append(diff.delHy2, old.Password)
			}
			m.closeLimiterLocked(id)
			continue
		}
		if !sameCredentials(old, nu) {
			if old.UUID != "" {
				diff.delVless = append(diff.delVless, old.UUID)
			}
			if old.Password != "" {
				diff.delHy2 = append(diff.delHy2, old.Password)
			}
		}
	}
	for id, nu := range next {
		old, ok := m.users[id]
		if ok && sameCredentials(old, nu) {
			m.updateLimiterLocked(nu)
			continue
		}
		if nu.UUID != "" {
			diff.addVless = append(diff.addVless, vlessUserFromPanelUser(nu))
		}
		if nu.Password != "" {
			name := nu.Name
			if name == "" {
				name = nu.Password
			}
			diff.addHy2 = append(diff.addHy2, option.Hysteria2User{Name: name, Password: nu.Password})
			diff.addHy2IDs = append(diff.addHy2IDs, nu.ID)
		}
		m.updateLimiterLocked(nu)
	}
	return diff
}

// diffUsers returns how many inbound add/delete entries a user sync would
// produce. It is used by tests to verify speed-limit-only changes do not
// re-add existing protocol users.
func (m *UserManager) diffUsers(users []panelapi.User) (adds int, dels int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := make(map[int]panelapi.User, len(users))
	for _, u := range users {
		if u.ID > 0 {
			next[u.ID] = u
		}
	}
	diff := m.diffUsersLocked(next)
	return len(diff.addVless) + len(diff.addHy2), len(diff.delVless) + len(diff.delHy2)
}

func (m *UserManager) updateLimiterLocked(u panelapi.User) {
	if !userRateLimitBuildEnabled || u.SpeedLimit <= 0 {
		m.closeLimiterLocked(u.ID)
		return
	}
	bytesPerSecond := mbpsToBytes(u.SpeedLimit)
	if l, ok := m.limiters[u.ID]; ok {
		l.SetRate(bytesPerSecond)
		return
	}
	p := newLimiterPair(bytesPerSecond)
	m.limiters[u.ID] = &p
}

func (m *UserManager) closeLimiterLocked(id int) {
	if l, ok := m.limiters[id]; ok {
		l.Close()
		delete(m.limiters, id)
	}
}

func (m *UserManager) Resolve(user string) string {
	if user == "" {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.bySecret[user]; ok {
		return fmt.Sprint(id)
	}
	return user
}

func (m *UserManager) ActiveIDs() map[string]struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]struct{}, len(m.users))
	for id := range m.users {
		out[fmt.Sprint(id)] = struct{}{}
	}
	return out
}

// DirectionalLimiters returns separate read and write limiters for both the
// node level and the per-user level. Each direction gets its own token bucket
// so that saturated download traffic cannot starve upload ACKs (or vice versa).
func (m *UserManager) DirectionalLimiters(user string) (nodeRead, nodeWrite, userRead, userWrite *counter.RateLimiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nodeLimit != nil {
		nodeRead = m.nodeLimit.rx
		nodeWrite = m.nodeLimit.tx
	}
	if userRateLimitBuildEnabled {
		if id, ok := m.bySecret[user]; ok {
			if p := m.limiters[id]; p != nil {
				userRead = p.rx
				userWrite = p.tx
			}
		}
	}
	return
}

// Limiters returns a representative limiter for inspection / testing.
// Deprecated: prefer DirectionalLimiters for connection wiring.
func (m *UserManager) Limiters(user string) (*counter.RateLimiter, *counter.RateLimiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var nodeLim, userLim *counter.RateLimiter
	if m.nodeLimit != nil {
		nodeLim = m.nodeLimit.rx
	}
	if userRateLimitBuildEnabled {
		if id, ok := m.bySecret[user]; ok {
			if p := m.limiters[id]; p != nil {
				userLim = p.rx
			}
		}
	}
	return nodeLim, userLim
}
