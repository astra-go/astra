package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/astra-go/astra/e2e/testapp"
)

func TestHTTP_Register_Success(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	body := mustMarshal(t, map[string]string{"username": "bob", "password": "bobpass1"})
	resp := doJSON(t, client, http.MethodPost, base+"/auth/register", body)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var out map[string]any
	mustDecodeJSON(t, resp, &out)
	if out["username"] != "bob" {
		t.Errorf("want username=bob, got %v", out["username"])
	}
}

func TestHTTP_Register_ShortPassword(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	body := mustMarshal(t, map[string]string{"username": "carol", "password": "abc"})
	resp := doJSON(t, client, http.MethodPost, base+"/auth/register", body)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestHTTP_Register_ShortUsername(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	body := mustMarshal(t, map[string]string{"username": "ab", "password": "validpass"})
	resp := doJSON(t, client, http.MethodPost, base+"/auth/register", body)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestHTTP_Login_WrongPassword(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Register first.
	body := mustMarshal(t, map[string]string{"username": "dave", "password": "davepass1"})
	resp := doJSON(t, client, http.MethodPost, base+"/auth/register", body)
	resp.Body.Close()

	// Login with wrong password.
	body = mustMarshal(t, map[string]string{"username": "dave", "password": "wrongpass"})
	resp = doJSON(t, client, http.MethodPost, base+"/auth/login", body)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestHTTP_Login_UnknownUser(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	body := mustMarshal(t, map[string]string{"username": "nobody", "password": "somepass"})
	resp := doJSON(t, client, http.MethodPost, base+"/auth/login", body)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestHTTP_Me_InvalidToken(t *testing.T) {
	app := testapp.New(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestHTTP_Me_TokenInResponse(t *testing.T) {
	app := testapp.New(t)
	token := registerAndLogin(t, app, "eve", "evepass1")

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, app.HTTPURL()+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.HTTP.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var out map[string]any
	mustDecodeJSON(t, resp, &out)
	if out["username"] != "eve" {
		t.Errorf("want username=eve, got %v", out["username"])
	}
	if _, ok := out["id"]; !ok {
		t.Error("response missing 'id' field")
	}
}
