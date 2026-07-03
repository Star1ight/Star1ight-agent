# Star1ight Agent SS/SS2022 Support Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add lightweight native XBoard-compatible `SS/SS2022` support to `Star1ight agent`, including config generation, runtime protocol registration, user sync, and traffic attribution.

**Architecture:** Keep XBoard as the source of truth and extend the existing lightweight adapter only where needed. The implementation adds `shadowsocks` support across four layers: panel config decoding, sing-box config generation, runtime protocol registration, and user sync / traffic push integration. Existing `VLESS/HY2` behavior must remain unchanged.

**Tech Stack:** Go 1.25, sing-box `v1.13.0` fork, XBoard UniProxy API, Go test

---

## Chunk 1: XBoard Config Contract

### Task 1: Add failing node-config decoding tests

**Files:**
- Modify: `panelapi/node_config_test.go`
- Modify: `panelapi/node_config.go`

- [ ] **Step 1: Write the failing test**

Add a test that serves a native XBoard `shadowsocks` payload and asserts:

```go
cfg.Protocol == "shadowsocks"
cfg.ServerPort == 3001
cfg.Cipher == "2022-blake3-aes-256-gcm"
cfg.ServerKey != ""
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./panelapi -run TestFetchNodeConfigReadsShadowsocksFields -v`
Expected: FAIL because `NodeConfig` does not yet expose `Cipher` / `ServerKey`

- [ ] **Step 3: Write minimal implementation**

Extend `panelapi.NodeConfig` with:

```go
Cipher    string `json:"cipher,omitempty"`
ServerKey string `json:"server_key,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./panelapi -run TestFetchNodeConfigReadsShadowsocksFields -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add panelapi/node_config.go panelapi/node_config_test.go
git commit -m "test: cover xboard shadowsocks config decoding"
```

## Chunk 2: Config Generation And Runtime Registration

### Task 2: Add failing config-generation tests for SS/SS2022

**Files:**
- Modify: `cmd/star1ight-agent/xboard_config_test.go`
- Modify: `cmd/star1ight-agent/xboard_config.go`

- [ ] **Step 1: Write the failing test**

Add a test that:

- stubs `/api/v1/server/UniProxy/config` with `protocol=shadowsocks`
- uses `generateXboardConfig(... NodeMode: "ss" ...)`
- asserts the generated config loads through `loadOptions`
- asserts the inbound type is `shadowsocks`

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/star1ight-agent -run TestGenerateConfigFromXboardSS2022 -v`
Expected: FAIL because `ss` mode and `shadowsocks` config generation are unsupported

- [ ] **Step 3: Write minimal implementation**

Add:

- `normalizeNodeMode("ss"|"shadowsocks")`
- `case "ss":` in `generateXboardConfig`
- `case "shadowsocks":` in `inboundFromNodeConfig`
- helper to build a sing-box `shadowsocks` inbound with tag `ss-in`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/star1ight-agent -run TestGenerateConfigFromXboardSS2022 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/star1ight-agent/xboard_config.go cmd/star1ight-agent/xboard_config_test.go
git commit -m "feat: generate shadowsocks node configs"
```

### Task 3: Add failing runtime protocol-scope test

**Files:**
- Modify: `cmd/star1ight-agent/protocol_scope_test.go`
- Modify: `cmd/star1ight-agent/main.go`

- [ ] **Step 1: Write the failing test**

Add `shadowsocks` to the accepted lightweight protocol set with a minimal loadable inbound config:

```json
{
  "type": "shadowsocks",
  "tag": "ss-in",
  "listen": "127.0.0.1",
  "listen_port": 0,
  "method": "2022-blake3-aes-256-gcm",
  "password": "server-key"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/star1ight-agent -run TestLoadOptionsAcceptsCoreNodeProtocols -v`
Expected: FAIL because `minimalContext()` does not register `shadowsocks`

- [ ] **Step 3: Write minimal implementation**

Register sing-box `shadowsocks` inbound/outbound in `minimalContext()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/star1ight-agent -run TestLoadOptionsAcceptsCoreNodeProtocols -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/star1ight-agent/main.go cmd/star1ight-agent/protocol_scope_test.go
git commit -m "feat: register shadowsocks runtime support"
```

## Chunk 3: User Sync And Traffic Attribution

### Task 4: Add failing user-manager tests for SS multi-user sync

**Files:**
- Modify: `cmd/star1ight-agent/user_manager_test.go`
- Modify: `cmd/star1ight-agent/user_manager.go`

- [ ] **Step 1: Write the failing test**

Add a test covering:

- an inbound map containing a `*shadowsocks.MultiInbound`
- two XBoard users with numeric ids and per-user `Password`
- one subsequent sync that removes one user

Assert:

- users are added with stable names
- removed users disappear on resync
- `Resolve()` maps SS secrets back to numeric user ids

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/star1ight-agent -run TestUserManagerApplyBoxShadowsocksUsers -v`
Expected: FAIL because `ApplyBox()` does not handle `shadowsocks.MultiInbound`

- [ ] **Step 3: Write minimal implementation**

Add a `shadowsocks` branch inside `ApplyBox()` that:

- converts panel users to `option.ShadowsocksUser`
- uses numeric id strings as `Name`
- uses `panelapi.User.Password` as per-user password
- deletes removed users by the same stable name

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/star1ight-agent -run TestUserManagerApplyBoxShadowsocksUsers -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/star1ight-agent/user_manager.go cmd/star1ight-agent/user_manager_test.go
git commit -m "feat: sync shadowsocks users from xboard"
```

### Task 5: Add failing traffic-match tests for shadowsocks nodes

**Files:**
- Modify: `panelapi/client_fetchusers_test.go` or add a focused client test file
- Modify: `panelapi/client.go`

- [ ] **Step 1: Write the failing test**

Add a focused test that verifies:

```go
NewClient(..., "shadowsocks").matchesInbound("ss-in") == true
```

and that `PushTraffic()` includes traffic from `ss-in`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./panelapi -run 'TestClientMatchesShadowsocksInbound|TestPushTrafficIncludesShadowsocksInbound' -v`
Expected: FAIL because `matchesInbound()` does not know `ss-in`

- [ ] **Step 3: Write minimal implementation**

Extend `matchesInbound()` with:

- `ss`, `shadowsocks`, and optionally `ss2022` aliases
- explicit `ss-in` match

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./panelapi -run 'TestClientMatchesShadowsocksInbound|TestPushTrafficIncludesShadowsocksInbound' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add panelapi/client.go panelapi/client_fetchusers_test.go
git commit -m "feat: attribute shadowsocks traffic to xboard"
```

## Chunk 4: Final Verification

### Task 6: Run the full verification suite

**Files:**
- Verify only

- [ ] **Step 1: Run targeted package tests**

Run: `go test ./panelapi ./cmd/star1ight-agent -v`
Expected: PASS

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Build the binary**

Run: `go build ./cmd/star1ight-agent`
Expected: build succeeds with native `SS/SS2022` support included

- [ ] **Step 4: Sanity-check no VLESS/HY2 regressions**

Re-run:

```bash
go test ./cmd/star1ight-agent -run 'TestGenerateConfigFromXboardVLESS|TestGenerateConfigFromXboardHY2|TestGenerateConfigFromXboardBothVLESSAndHY2' -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: add native shadowsocks support"
```

Plan complete and saved to `docs/superpowers/plans/2026-07-01-star1ight-agent-ss-support.md`. Ready to execute?
