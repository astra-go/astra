package e2e_test

import (
	"testing"
	"time"

	"github.com/astra-go/astra/e2e/testapp"
	gorilla "github.com/gorilla/websocket"
)

func TestWebSocket_Echo(t *testing.T) {
	app := testapp.New(t)
	token := registerAndLogin(t, app, "ws_user", "wspass123")

	wsURL := "ws" + app.HTTPURL()[len("http"):] + "/ws?token=" + token
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	msgs := []string{`{"event":"ping"}`, `{"event":"data","payload":"hello"}`, `plain text`}
	for _, msg := range msgs {
		if err := conn.WriteMessage(gorilla.TextMessage, []byte(msg)); err != nil {
			t.Fatalf("ws write %q: %v", msg, err)
		}
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ws read after %q: %v", msg, err)
		}
		if string(got) != msg {
			t.Errorf("ws echo: sent %q, got %q", msg, got)
		}
	}
}

func TestWebSocket_NoToken(t *testing.T) {
	app := testapp.New(t)

	wsURL := "ws" + app.HTTPURL()[len("http"):] + "/ws"
	_, resp, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail without token")
	}
	// The server should reject the upgrade with 401.
	if resp != nil && resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebSocket_InvalidToken(t *testing.T) {
	app := testapp.New(t)

	wsURL := "ws" + app.HTTPURL()[len("http"):] + "/ws?token=bad.token.here"
	_, resp, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail with invalid token")
	}
	if resp != nil && resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebSocket_MultipleClients(t *testing.T) {
	app := testapp.New(t)

	// Two independent users, each with their own connection.
	token1 := registerAndLogin(t, app, "ws_multi1", "multipass1")
	token2 := registerAndLogin(t, app, "ws_multi2", "multipass2")

	base := "ws" + app.HTTPURL()[len("http"):]
	conn1, _, err := gorilla.DefaultDialer.Dial(base+"/ws?token="+token1, nil)
	if err != nil {
		t.Fatalf("conn1 dial: %v", err)
	}
	defer conn1.Close()

	conn2, _, err := gorilla.DefaultDialer.Dial(base+"/ws?token="+token2, nil)
	if err != nil {
		t.Fatalf("conn2 dial: %v", err)
	}
	defer conn2.Close()

	// Each client sends a unique message and receives only its own echo.
	send1 := []byte(`{"from":"client1"}`)
	send2 := []byte(`{"from":"client2"}`)

	if err := conn1.WriteMessage(gorilla.TextMessage, send1); err != nil {
		t.Fatalf("conn1 write: %v", err)
	}
	if err := conn2.WriteMessage(gorilla.TextMessage, send2); err != nil {
		t.Fatalf("conn2 write: %v", err)
	}

	conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, got1, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("conn1 read: %v", err)
	}
	if string(got1) != string(send1) {
		t.Errorf("conn1 echo: want %s, got %s", send1, got1)
	}

	conn2.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, got2, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("conn2 read: %v", err)
	}
	if string(got2) != string(send2) {
		t.Errorf("conn2 echo: want %s, got %s", send2, got2)
	}
}
