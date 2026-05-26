// Package fcm provides a Firebase Cloud Messaging (FCM) backend for the
// github.com/astra-go/astra/notify/push package.
//
// It calls the FCM HTTP v1 API (fcm.googleapis.com/v1) using a service-account
// OAuth2 Bearer token — no Firebase Admin SDK dependency required.
//
// # Authentication
//
// FCM HTTP v1 requires a Google OAuth2 access token scoped to
// "https://www.googleapis.com/auth/firebase.messaging".
// Supply the raw JSON service-account key file content in ServiceAccountJSON,
// and the sender will exchange it for short-lived access tokens automatically.
//
// Alternatively, supply a static BearerToken for testing or environments where
// token exchange is handled externally.
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/notify/push"
//	    pushfcm "github.com/astra-go/astra/notify/push/fcm"
//	)
//
//	sender, err := pushfcm.New(pushfcm.Config{
//	    ProjectID:          os.Getenv("FIREBASE_PROJECT_ID"),
//	    ServiceAccountJSON: serviceAccountBytes,
//	})
//
//	result, err := sender.Send(ctx, &push.Message{
//	    Token: deviceToken,
//	    Title: "Hello",
//	    Body:  "World",
//	    Data:  map[string]string{"key": "value"},
//	})
package fcm

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/astra-go/astra/notify/push"
)

// Config configures the FCM sender.
type Config struct {
	// ProjectID is the Firebase project ID (e.g. "my-project-123"). Required.
	ProjectID string

	// BearerToken is a pre-obtained OAuth2 Bearer token.
	// Use for testing or when token management is external.
	// Mutually exclusive with ServiceAccountJSON.
	BearerToken string

	// ServiceAccountJSON is the content of the Firebase service-account key
	// JSON file. The sender exchanges it for short-lived access tokens.
	// Mutually exclusive with BearerToken.
	ServiceAccountJSON []byte

	// HTTPTimeout sets the HTTP client timeout. Default: 10 seconds.
	HTTPTimeout time.Duration
}

// Sender implements push.Sender using the FCM HTTP v1 API.
type Sender struct {
	cfg       Config
	client    *http.Client
	tokenFunc func(ctx context.Context) (string, error)
}

// New creates an FCM Sender.
func New(cfg Config) (*Sender, error) {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	s := &Sender{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
	switch {
	case cfg.BearerToken != "":
		s.tokenFunc = func(_ context.Context) (string, error) { return cfg.BearerToken, nil }
	case len(cfg.ServiceAccountJSON) > 0:
		tf, err := newServiceAccountTokenFunc(cfg.ServiceAccountJSON, &http.Client{Timeout: 10 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("fcm: parse service account: %w", err)
		}
		s.tokenFunc = tf
	default:
		return nil, fmt.Errorf("fcm: BearerToken or ServiceAccountJSON is required")
	}
	return s, nil
}

// Send delivers a push notification to a single token or topic.
func (s *Sender) Send(ctx context.Context, msg *push.Message) (*push.SendResult, error) {
	token, err := s.tokenFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("fcm: get token: %w", err)
	}

	fcmMsg := buildFCMMessage(msg)
	body, _ := json.Marshal(map[string]any{"message": fcmMsg})

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.cfg.ProjectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fcm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fcm: send: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return &push.SendResult{Error: fmt.Errorf("fcm: HTTP %d: %s", resp.StatusCode, string(respBody))}, nil
	}

	var result struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(respBody, &result)
	return &push.SendResult{MessageID: result.Name}, nil
}

// SendBatch sends messages sequentially (FCM v1 has no free-tier batch endpoint).
func (s *Sender) SendBatch(ctx context.Context, msgs []*push.Message) ([]*push.SendResult, error) {
	results := make([]*push.SendResult, len(msgs))
	for i, m := range msgs {
		r, err := s.Send(ctx, m)
		if err != nil {
			results[i] = &push.SendResult{Error: err}
		} else {
			results[i] = r
		}
	}
	return results, nil
}

// Compile-time assertion.
var _ push.Sender = (*Sender)(nil)

// ─── FCM message builder ──────────────────────────────────────────────────────

func buildFCMMessage(msg *push.Message) map[string]any {
	notification := map[string]any{}
	if msg.Title != "" {
		notification["title"] = msg.Title
	}
	if msg.Body != "" {
		notification["body"] = msg.Body
	}
	if msg.ImageURL != "" {
		notification["image"] = msg.ImageURL
	}

	android := map[string]any{}
	if msg.Priority == "high" {
		android["priority"] = "HIGH"
	}
	if msg.TTL > 0 {
		android["ttl"] = fmt.Sprintf("%ds", msg.TTL)
	}
	if msg.CollapseKey != "" {
		android["collapse_key"] = msg.CollapseKey
	}

	fcmMsg := map[string]any{
		"notification": notification,
		"android":      android,
	}
	if len(msg.Data) > 0 {
		fcmMsg["data"] = msg.Data
	}
	if msg.Token != "" {
		fcmMsg["token"] = msg.Token
	} else if msg.Topic != "" {
		fcmMsg["topic"] = msg.Topic
	}
	return fcmMsg
}

// ─── service-account token exchange ──────────────────────────────────────────

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func newServiceAccountTokenFunc(saJSON []byte, hc *http.Client) (func(context.Context) (string, error), error) {
	var sa serviceAccount
	if err := json.Unmarshal(saJSON, &sa); err != nil {
		return nil, err
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("invalid service account JSON: missing client_email or private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}

	key, err := parseRSAPrivateKey([]byte(sa.PrivateKey))
	if err != nil {
		return nil, err
	}

	var (
		mu           sync.Mutex
		cachedToken  string
		cachedExpiry time.Time
	)

	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if cachedToken != "" && time.Now().Before(cachedExpiry) {
			return cachedToken, nil
		}
		tok, expiry, err := exchangeJWT(ctx, sa, key, hc)
		if err != nil {
			return "", err
		}
		cachedToken = tok
		cachedExpiry = expiry
		return tok, nil
	}, nil
}

func exchangeJWT(ctx context.Context, sa serviceAccount, key *rsa.PrivateKey, hc *http.Client) (string, time.Time, error) {
	now := time.Now()
	jwt, err := signRS256JWT(key, map[string]any{
		"iss":   sa.ClientEmail,
		"sub":   sa.ClientEmail,
		"aud":   sa.TokenURI,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fcm: sign JWT: %w", err)
	}

	reqBody := strings.NewReader(
		"grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + jwt,
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, reqBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hc.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fcm: token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", time.Time{}, fmt.Errorf("fcm: parse token: %w", err)
	}
	if tok.Error != "" {
		return "", time.Time{}, fmt.Errorf("fcm: token error: %s", tok.Error)
	}
	expiry := time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	return tok.AccessToken, expiry, nil
}

// ─── JWT / RSA helpers ────────────────────────────────────────────────────────

func signRS256JWT(key *rsa.PrivateKey, claims map[string]any) (string, error) {
	header := base64RawURL(mustJSON(map[string]any{"alg": "RS256", "typ": "JWT"}))
	payload := base64RawURL(mustJSON(claims))
	signingInput := header + "." + payload

	h := sha256.New()
	h.Write([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h.Sum(nil))
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64RawURL(sig), nil
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("fcm: failed to decode PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 as fallback.
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("fcm: private key is not RSA")
	}
	return rsaKey, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func base64RawURL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
