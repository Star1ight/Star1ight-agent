package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"star1ight-agent/panelapi"

	box "github.com/sagernet/sing-box"
	sbshadowsocks "github.com/sagernet/sing-box/protocol/shadowsocks"
)

const (
	testSS2022ServerPassword     = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	testSS2022FirstUserPassword  = "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU="
	testSS2022SecondUserPassword = "ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA="
)

func TestUserManagerResolvesAliasesAndActiveIDs(t *testing.T) {
	m := NewUserManager(0)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", Name: "name-7"}}); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"uuid-7", "pw-7", "name-7"} {
		if got := m.Resolve(secret); got != "7" {
			t.Fatalf("Resolve(%q)=%q, want 7", secret, got)
		}
	}
	active := m.ActiveIDs()
	if _, ok := active["7"]; !ok || len(active) != 1 {
		t.Fatalf("active ids mismatch: %#v", active)
	}
}

func TestUserManagerHotDeleteRemovesAliasesAndLimiter(t *testing.T) {
	m := NewUserManager(10)
	if err := m.ApplyBox(nil, []panelapi.User{{ID: 7, UUID: "uuid-7", Password: "pw-7", SpeedLimit: 3}}); err != nil {
		t.Fatal(err)
	}
	_, userLimiter := m.Limiters("uuid-7")
	if userRateLimitBuildEnabled && userLimiter == nil {
		t.Fatal("expected user limiter before delete")
	}
	if err := m.ApplyBox(nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := m.Resolve("uuid-7"); got != "uuid-7" {
		t.Fatalf("deleted alias still resolves to %q", got)
	}
	if active := m.ActiveIDs(); len(active) != 0 {
		t.Fatalf("expected no active ids, got %#v", active)
	}
	_, userLimiter = m.Limiters("uuid-7")
	if userLimiter != nil {
		t.Fatal("deleted user limiter still exists")
	}
}

func TestUserManagerApplyBoxShadowsocksUsers(t *testing.T) {
	boxInstance, ssInbound := mustNewManagedShadowsocksBox(t)
	defer func() {
		if err := boxInstance.Close(); err != nil {
			t.Fatalf("close box: %v", err)
		}
	}()

	m := NewUserManager(0)
	if err := m.ApplyBox(collectInbounds(boxInstance), []panelapi.User{
		{ID: 7, Password: testSS2022FirstUserPassword},
		{ID: 8, Password: testSS2022SecondUserPassword},
	}); err != nil {
		t.Fatal(err)
	}
	if got := shadowsocksInboundUserNames(ssInbound); !reflect.DeepEqual(got, []string{"7", "8"}) {
		t.Fatalf("shadowsocks users = %#v, want [7 8]", got)
	}
	if got := m.Resolve(testSS2022FirstUserPassword); got != "7" {
		t.Fatalf("Resolve(%q)=%q, want 7", testSS2022FirstUserPassword, got)
	}

	if err := m.ApplyBox(collectInbounds(boxInstance), []panelapi.User{
		{ID: 8, Password: testSS2022SecondUserPassword},
	}); err != nil {
		t.Fatal(err)
	}
	if got := shadowsocksInboundUserNames(ssInbound); !reflect.DeepEqual(got, []string{"8"}) {
		t.Fatalf("shadowsocks users after delete = %#v, want [8]", got)
	}
	if got := m.Resolve(testSS2022FirstUserPassword); got != testSS2022FirstUserPassword {
		t.Fatalf("deleted password still resolves to %q", got)
	}
}

func mustNewManagedShadowsocksBox(t *testing.T) (*box.Box, *sbshadowsocks.MultiInbound) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [
    {
      "type": "shadowsocks",
      "tag": "ss-in",
      "listen": "127.0.0.1",
      "listen_port": 0,
      "method": "2022-blake3-aes-256-gcm",
      "password": "` + testSS2022ServerPassword + `",
      "managed": true
    }
  ],
  "outbounds": [{"type": "direct", "tag": "direct"}],
  "route": {"final": "direct"}
}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	opts, err := loadOptions(path)
	if err != nil {
		t.Fatalf("loadOptions: %v", err)
	}
	boxInstance, err := box.New(box.Options{
		Context: minimalContext(context.Background()),
		Options: opts,
	})
	if err != nil {
		t.Fatalf("box.New: %v", err)
	}
	raw := collectInbounds(boxInstance)["ss-in"]
	ssInbound, ok := raw.(*sbshadowsocks.MultiInbound)
	if !ok {
		t.Fatalf("ss-in inbound type = %T, want *shadowsocks.MultiInbound", raw)
	}
	return boxInstance, ssInbound
}

func shadowsocksInboundUserNames(inbound *sbshadowsocks.MultiInbound) []string {
	usersField := reflect.ValueOf(inbound).Elem().FieldByName("users")
	names := make([]string, 0, usersField.Len())
	for i := 0; i < usersField.Len(); i++ {
		names = append(names, usersField.Index(i).FieldByName("Name").String())
	}
	return names
}
