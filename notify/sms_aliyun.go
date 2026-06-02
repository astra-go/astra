//go:build sms
// +build sms

package notify

// This file provides the Aliyun SMS sender, enabled with build tag "sms".

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
)

const aliyunSMSEndpoint = "https://dysmsapi.aliyuncs.com/"

// SmsAliyunConfig holds Aliyun SMS service credentials.
type SmsAliyunConfig struct {
	// AccessKeyID is the Aliyun RAM AccessKey ID. Required.
	AccessKeyID string

	// AccessKeySecret is the Aliyun RAM AccessKey Secret. Required.
	AccessKeySecret string

	// SignName is the SMS signature name registered in the Aliyun console.
	SignName string

	// TemplateCode is the SMS template code registered in the Aliyun console.
	TemplateCode string

	// HTTPTimeout sets the HTTP client timeout. Default: 10 seconds.
	HTTPTimeout time.Duration
}

// SmsAliyunSender implements SmsSender using the Aliyun SMS API.
type SmsAliyunSender struct {
	cfg    SmsAliyunConfig
	client *http.Client
}

// NewSmsAliyunSender creates an Aliyun SMS Sender.
func NewSmsAliyunSender(cfg SmsAliyunConfig) *SmsAliyunSender {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &SmsAliyunSender{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

// Send delivers an SMS via the Aliyun SMS API.
func (s *SmsAliyunSender) Send(ctx context.Context, msg *SmsMessage) error {
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

	params := map[string]string{
		"Action":           "SendSms",
		"Version":          "2017-05-25",
		"Format":           "JSON",
		"AccessKeyId":      s.cfg.AccessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   aliyunRandomNonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"PhoneNumbers":     msg.To,
		"SignName":         signName,
		"TemplateCode":     templateCode,
		"TemplateParam":    templateParam,
	}

	params["Signature"] = aliyunSign(s.cfg.AccessKeySecret, "GET", params)

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

// Verify SmsAliyunSender implements SmsSender at compile time.
var _ SmsSender = (*SmsAliyunSender)(nil)

func aliyunSign(secret, method string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	canonicalQuery := strings.Join(parts, "&")

	stringToSign := method + "&" + url.QueryEscape("/") + "&" + url.QueryEscape(canonicalQuery)

	mac := hmac.New(sha1.New, []byte(secret+"&")) //nolint:gosec
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func aliyunRandomNonce() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	return fmt.Sprintf("%d", n)
}
