// Package aliyun provides an Aliyun (Alibaba Cloud) SMS backend for the
// github.com/astra-go/astra/notify/sms package.
//
// It calls the Aliyun SMS OpenAPI (dysmsapi.aliyuncs.com) directly over HTTPS
// using standard library net/http — no Aliyun SDK dependency is required.
//
// # Authentication
//
// Requests are signed with HMAC-SHA1 using Signature Version 1.0 (query-string
// signing), which is the scheme used by all classic Aliyun APIs.
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/notify/sms"
//	    smsaliyun "github.com/astra-go/astra/notify/sms/aliyun"
//	)
//
//	sender := smsaliyun.New(smsaliyun.Config{
//	    AccessKeyID:     os.Getenv("ALIYUN_AK_ID"),
//	    AccessKeySecret: os.Getenv("ALIYUN_AK_SECRET"),
//	    SignName:        "MyApp",
//	    TemplateCode:    "SMS_123456789",
//	})
//
//	err := sender.Send(ctx, &sms.Message{
//	    To:     "+8613800138000",
//	    Params: map[string]string{"code": "998877"},
//	})
package aliyun

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // Aliyun API mandates SHA-1 signing
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/astra-go/astra/notify/sms"
)

const aliyunSMSEndpoint = "https://dysmsapi.aliyuncs.com/"

// Config holds Aliyun SMS service credentials.
type Config struct {
	// AccessKeyID is the Aliyun RAM AccessKey ID. Required.
	AccessKeyID string

	// AccessKeySecret is the Aliyun RAM AccessKey Secret. Required.
	AccessKeySecret string

	// SignName is the SMS signature name registered in the Aliyun console.
	// Used as default; can be overridden per message.
	SignName string

	// TemplateCode is the SMS template code registered in the Aliyun console.
	// Used as default; can be overridden per message.
	TemplateCode string

	// HTTPTimeout sets the HTTP client timeout. Default: 10 seconds.
	HTTPTimeout time.Duration
}

// Sender implements sms.Sender using the Aliyun SMS API.
type Sender struct {
	cfg    Config
	client *http.Client
}

// New creates an Aliyun SMS Sender.
func New(cfg Config) *Sender {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Sender{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

// Send delivers an SMS via the Aliyun SMS API.
func (s *Sender) Send(ctx context.Context, msg *sms.Message) error {
	signName := msg.SignName
	if signName == "" {
		signName = s.cfg.SignName
	}
	templateCode := msg.TemplateCode
	if templateCode == "" {
		templateCode = s.cfg.TemplateCode
	}

	templateParam := "{}"
	if len(msg.Params) > 0 {
		b, _ := json.Marshal(msg.Params)
		templateParam = string(b)
	}

	// Build common Aliyun API query parameters.
	params := map[string]string{
		"Action":           "SendSms",
		"Version":          "2017-05-25",
		"Format":           "JSON",
		"AccessKeyId":      s.cfg.AccessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   randomNonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"PhoneNumbers":     msg.To,
		"SignName":         signName,
		"TemplateCode":     templateCode,
		"TemplateParam":    templateParam,
	}

	// Sign the request.
	params["Signature"] = aliyunSign(s.cfg.AccessKeySecret, "GET", params)

	// Build the URL.
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	reqURL := aliyunSMSEndpoint + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("sms/aliyun: build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sms/aliyun: send: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("sms/aliyun: parse response: %w", err)
	}
	if result.Code != "OK" {
		return fmt.Errorf("sms/aliyun: API error %s: %s", result.Code, result.Message)
	}
	return nil
}

// Compile-time assertion.
var _ sms.Sender = (*Sender)(nil)

// ─── signing helpers ──────────────────────────────────────────────────────────

// aliyunSign computes the HMAC-SHA1 query-string signature required by Aliyun APIs.
func aliyunSign(secret, method string, params map[string]string) string {
	// 1. Sort parameters by key.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 2. Percent-encode and join as key=value pairs.
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	canonicalQuery := strings.Join(parts, "&")

	// 3. Build the string-to-sign.
	stringToSign := method + "&" + url.QueryEscape("/") + "&" + url.QueryEscape(canonicalQuery)

	// 4. HMAC-SHA1 with secret+"&".
	mac := hmac.New(sha1.New, []byte(secret+"&")) //nolint:gosec
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func randomNonce() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	return fmt.Sprintf("%d", n)
}
