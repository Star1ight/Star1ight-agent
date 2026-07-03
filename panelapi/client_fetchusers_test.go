package panelapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUsersFallbackAndFillMissingPasswordAndName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":101,"uuid":"uuid-101","speed_limit":7}]}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", "1", "vless")
	users, err := c.FetchUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("users len=%d, want 1", len(users))
	}
	if users[0].Password != "uuid-101" {
		t.Fatalf("password=%q, want uuid fallback", users[0].Password)
	}
	if users[0].Name != "101" {
		t.Fatalf("name=%q, want numeric id fallback", users[0].Name)
	}
}

func TestFetchUsersDerivesShadowsocks2022PasswordFromUUID(t *testing.T) {
	const uuid = "12345678-1234-1234-1234-1234567890ab"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/server/UniProxy/user":
			_, _ = w.Write([]byte(`{"users":[{"id":101,"uuid":"` + uuid + `","speed_limit":7}]}`))
		case "/api/v1/server/UniProxy/config":
			_, _ = w.Write([]byte(`{
				"protocol":"shadowsocks",
				"listen_ip":"0.0.0.0",
				"server_port":3001,
				"cipher":"2022-blake3-aes-256-gcm",
				"server_key":"server-key"
			}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, "token", "34", "shadowsocks")
	users, err := c.FetchUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("users len=%d, want 1", len(users))
	}
	wantPassword := base64.StdEncoding.EncodeToString([]byte(uuid[:32]))
	if users[0].Password != wantPassword {
		t.Fatalf("password=%q, want %q", users[0].Password, wantPassword)
	}
	if users[0].Name != "101" {
		t.Fatalf("name=%q, want numeric id fallback", users[0].Name)
	}
}
