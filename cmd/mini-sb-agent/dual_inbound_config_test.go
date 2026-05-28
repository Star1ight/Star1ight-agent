package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOptionsAcceptsSingleConfigWithVLESSAndHY2ProductionTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{
  "log": {"disabled": true},
  "inbounds": [
    {
      "type": "vless",
      "tag": "vless-tcp",
      "listen": "127.0.0.1",
      "listen_port": 0,
      "users": []
    },
    {
      "type": "hysteria2",
      "tag": "hy2-udp",
      "listen": "127.0.0.1",
      "listen_port": 0,
      "users": [],
      "tls": {"enabled": true, "server_name": "example.com", "certificate_path": "/tmp/missing.crt", "key_path": "/tmp/missing.key"}
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
		t.Fatalf("load single-process dual inbound config: %v", err)
	}
	if len(opts.Inbounds) != 2 {
		t.Fatalf("inbounds = %d, want 2", len(opts.Inbounds))
	}
	if opts.Inbounds[0].Tag != "vless-tcp" || opts.Inbounds[1].Tag != "hy2-udp" {
		t.Fatalf("inbound tags = %q/%q, want vless-tcp/hy2-udp", opts.Inbounds[0].Tag, opts.Inbounds[1].Tag)
	}
}
