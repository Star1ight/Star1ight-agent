# Star1ight Agent Native SS/SS2022 Support Design

## Goal

Keep `Star1ight agent` lightweight while adding native XBoard-compatible `SS` and `SS2022` support to the existing machine/node workflow.

The agent should be able to:

- fetch an XBoard `shadowsocks` node config
- generate a valid sing-box `shadowsocks` inbound from it
- register the `shadowsocks` protocol in the runtime build
- sync XBoard users into a multi-user `SS/SS2022` inbound
- attribute traffic back to the correct XBoard users

## Non-Goals

- no XBoard server-side changes
- no compatibility with the current production custom `server_password + derived client password` migration model in this round
- no legacy `plugin` or old obfs compatibility unless XBoard native payload already requires it
- no broad protocol expansion beyond the current scope of `VLESS`, `HY2`, `TUIC`, `SSH`, and new `SS/SS2022`

## Current Problem

The current fork already works for `VLESS` and `HY2`, but `SS/SS2022` is blocked in three places:

1. `cmd/star1ight-agent/main.go`
   The minimal runtime context does not register the sing-box `shadowsocks` protocol.
2. `panelapi/node_config.go`
   The XBoard node config struct does not model the key `shadowsocks` fields needed for sing-box generation.
3. `cmd/star1ight-agent/xboard_config.go`
   Config generation only supports `vless` and `hysteria/hysteria2`.

There is also a fourth practical gap for real node mode:

4. `cmd/star1ight-agent/user_manager.go` and `panelapi/client.go`
   The user sync path only knows how to add/remove `VLESS` and `HY2` users. Even if `SS/SS2022` config loads, it will not behave like a real XBoard-managed node without user sync and inbound matching.

## Recommended Approach

Implement native XBoard `SS/SS2022` support inside the existing lightweight fork, without adding a custom control-plane layer.

This keeps the architecture aligned with upstream XBoard contracts:

- XBoard remains the source of truth for node config and users.
- `Star1ight agent` remains a small execution/runtime adapter.
- sing-box remains the protocol engine.

## Data Contract

The agent should accept XBoard `shadowsocks` config in the native shape already exposed by the panel, specifically:

- `protocol=shadowsocks`
- `server_port`
- `cipher`
- `server_key` for `SS2022`
- optional `network` and `listen_ip`

For runtime user sync, the existing XBoard user endpoint should continue to provide per-user secrets through `panelapi.User.Password`. The agent should treat that password as the per-user `SS/SS2022` credential.

## Design

### 1. Runtime Protocol Registration

Update `minimalContext()` in `cmd/star1ight-agent/main.go` to register sing-box `shadowsocks` support in the inbound registry and outbound registry.

This is required so generated configs can actually be loaded by the lightweight build.

### 2. XBoard Config Decoding

Extend `panelapi.NodeConfig` with the minimal `SS/SS2022` fields required by sing-box config generation:

- `Cipher string`
- `ServerKey string`

Keep the struct narrow. Do not add speculative fields that are not needed by the current XBoard payload or this implementation.

### 3. Config Generation

Extend `inboundFromNodeConfig()` in `cmd/star1ight-agent/xboard_config.go` with a `shadowsocks` branch.

Behavior:

- map XBoard `cipher` to sing-box inbound `method`
- use XBoard `server_key` as the sing-box inbound `password`
- emit a multi-user inbound by default so panel users can be synced dynamically
- keep the inbound tag stable as `ss-in`
- respect `listen_ip`, `server_port`, and `network`

This design intentionally targets XBoard-native `SS/SS2022` rather than the current production custom password-composition model.

### 4. Node-Mode CLI Surface

Extend the generator CLI so `xboard-generate-config` can explicitly build `SS/SS2022` node configs.

Expected additions:

- support `--node-mode ss`
- accept `panel_node_type=shadowsocks`

Existing `vless`, `hy2`, and `both` behavior should remain unchanged.

`both` should remain limited to the current `VLESS + HY2` dual-node layout for now. We are not introducing a generic multi-protocol composition layer in this round.

### 5. User Sync

Extend `UserManager.ApplyBox()` to support sing-box `shadowsocks.MultiInbound`.

Behavior:

- add `panelapi.User.Password` as the SS user password
- use a stable per-user name for deletion and traffic attribution
- remove absent users cleanly

For consistency with the existing traffic pipeline, the preferred user identity is the numeric XBoard user id rendered as a string.

### 6. Traffic Attribution

Update the inbound matching logic in `panelapi.Client.matchesInbound()` so a `shadowsocks` node pushes traffic for `ss-in`.

This ensures the existing `PushTraffic()` flow can attribute SS traffic to the correct XBoard users without changing the upstream panel contract.

## Error Handling

The implementation should fail closed for malformed `SS/SS2022` node configs:

- missing `server_port`
- missing `cipher`
- missing `server_key`
- unsupported or empty method

Failures should happen during config generation or config loading, not later during live traffic.

## Testing Strategy

Follow TDD for the new behavior.

Required tests:

1. `panelapi/node_config_test.go`
   Verify native XBoard `shadowsocks` payloads decode into the new fields.
2. `cmd/star1ight-agent/xboard_config_test.go`
   Verify `--node-mode ss` generates a valid sing-box config for `SS2022`.
3. `cmd/star1ight-agent/protocol_scope_test.go`
   Verify the lightweight build now accepts `shadowsocks` inbound config.
4. `cmd/star1ight-agent/user_manager_test.go`
   Verify user add/remove behavior for `shadowsocks.MultiInbound`.
5. `panelapi/client`-level tests
   Verify traffic matching includes `node_type=shadowsocks`.

Verification should also include a real `go test ./...` run and a local build of the binary.

## Risks

- XBoard may return slightly different field names for some `SS` variants; tests should pin the observed native payload shape.
- sing-box `shadowsocks` multi-user mode expects a server password plus per-user passwords; this is compatible with native XBoard `SS2022`, but not with the current production custom composition path.
- expanding user sync for `SS` must not regress existing `VLESS/HY2` behavior.

## Rollout

First deliver native `SS/SS2022` support in the fork and validate it locally.

If that passes, the next phase can evaluate whether production migration needs a separate compatibility layer for the current custom `SS2022` password model. That second phase should be a separate design and should not be mixed into this lightweight native-support patch.
