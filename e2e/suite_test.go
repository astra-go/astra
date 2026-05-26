// Package e2e contains end-to-end tests for the Astra framework.
//
// The suite exercises the full request lifecycle against a real in-process
// server: registration → login → protected API → WebSocket → gRPC.
//
// Run with:
//
//	go test -v -race ./e2e/...
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/astra-go/astra/e2e/testapp"
	gorilla "github.com/gorilla/websocket"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestE2EFullFlow is the primary end-to-end scenario:
// register → login → protected API → WebSocket echo → gRPC echo.
func TestE2EFullFlow(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// ── 1. Register ──────────────────────────────────────────────────────────
	t.Run("register", func(t *testing.T) {
		body := mustMarshal(t, map[string]string{"username": "alice", "password": "secret123"})
		resp := doJSON(t, client, http.MethodPost, base+"/auth/register", body)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		var out map[string]any
		mustDecodeJSON(t, resp, &out)
		if out["username"] != "alice" {
			t.Errorf("register: want username=alice, got %v", out["username"])
		}
		if out["id"] == "" {
			t.Error("register: expected non-empty id")
		}
	})

	// ── 2. Duplicate registration → 409 ──────────────────────────────────────
	t.Run("register_duplicate", func(t *testing.T) {
		body := mustMarshal(t, map[string]string{"username": "alice", "password": "otherpass1"})
		resp := doJSON(t, client, http.MethodPost, base+"/auth/register", body)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusConflict)
	})

	// ── 3. Login ─────────────────────────────────────────────────────────────
	var token string
	t.Run("login", func(t *testing.T) {
		body := mustMarshal(t, map[string]string{"username": "alice", "password": "secret123"})
		resp := doJSON(t, client, http.MethodPost, base+"/auth/login", body)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		var out map[string]any
		mustDecodeJSON(t, resp, &out)
		tok, ok := out["token"].(string)
		if !ok || tok == "" {
			t.Fatal("login: expected non-empty token")
		}
		token = tok
	})

	// ── 4. Protected API without token → 401 ─────────────────────────────────
	t.Run("api_me_no_token", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/me", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusUnauthorized)
	})

	// ── 5. Protected API with valid token → 200 ───────────────────────────────
	t.Run("api_me_with_token", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		var out map[string]any
		mustDecodeJSON(t, resp, &out)
		if out["username"] != "alice" {
			t.Errorf("api/me: want username=alice, got %v", out["username"])
		}
	})

	// ── 6. WebSocket echo ─────────────────────────────────────────────────────
	t.Run("websocket_echo", func(t *testing.T) {
		wsURL := "ws" + base[len("http"):] + "/ws?token=" + token
		conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("ws dial: %v", err)
		}
		defer conn.Close()

		want := []byte(`{"msg":"hello"}`)
		if err := conn.WriteMessage(gorilla.TextMessage, want); err != nil {
			t.Fatalf("ws write: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ws read: %v", err)
		}
		if string(got) != string(want) {
			t.Errorf("ws echo: want %s, got %s", want, got)
		}
	})

	// ── 7. gRPC Echo with valid token ─────────────────────────────────────────
	t.Run("grpc_echo_authed", func(t *testing.T) {
		conn := app.GRPCConn(t)
		resp, err := grpcEcho(conn, "hello grpc", token)
		if err != nil {
			t.Fatalf("grpc echo: %v", err)
		}
		if resp.Message != "hello grpc" {
			t.Errorf("grpc echo: want 'hello grpc', got %q", resp.Message)
		}
	})

	// ── 8. gRPC Echo without token → Unauthenticated ──────────────────────────
	t.Run("grpc_echo_no_token", func(t *testing.T) {
		conn := app.GRPCConn(t)
		_, err := grpcEcho(conn, "hello", "")
		if err == nil {
			t.Fatal("grpc echo without token: expected error")
		}
		s, ok := status.FromError(err)
		if !ok || s.Code() != codes.Unauthenticated {
			t.Errorf("grpc echo: want Unauthenticated, got %v", err)
		}
	})
}

// ── shared test helpers ───────────────────────────────────────────────────────

// registerAndLoginURL registers a new user against baseURL and returns the JWT token.
func registerAndLoginURL(t testing.TB, client *http.Client, baseURL, username, password string) string {
	t.Helper()

	body := mustMarshal(t, map[string]string{"username": username, "password": password})
	resp := doJSON(t, client, http.MethodPost, baseURL+"/auth/register", body)
	resp.Body.Close()

	body = mustMarshal(t, map[string]string{"username": username, "password": password})
	resp = doJSON(t, client, http.MethodPost, baseURL+"/auth/login", body)
	defer resp.Body.Close()

	var out map[string]any
	mustDecodeJSON(t, resp, &out)
	tok, ok := out["token"].(string)
	if !ok || tok == "" {
		t.Fatal("registerAndLogin: expected non-empty token")
	}
	return tok
}

// registerAndLogin is a convenience wrapper for *testapp.App.
func registerAndLogin(t testing.TB, app *testapp.App, username, password string) string {
	t.Helper()
	return registerAndLoginURL(t, app.HTTP.Client(), app.HTTPURL(), username, password)
}

func mustMarshal(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func doJSON(t testing.TB, client *http.Client, method, url string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("status: want %d, got %d", want, resp.StatusCode)
	}
}

func mustDecodeJSON(t testing.TB, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}
