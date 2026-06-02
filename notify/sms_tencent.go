//go:build sms
// +build sms

package notify

// This file provides the Tencent Cloud SMS sender, enabled with build tag "sms".

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	tcSMSEndpoint = "https://sms.tencentcloudapi.com"
	tcSMSService  = "sms"
	tcSMSVersion  = "2021-01-11"
	tcSMSAction   = "SendSms"
)

// SmsTencentConfig holds Tencent Cloud SMS credentials and defaults.
type SmsTencentConfig struct {
	// SecretID and SecretKey are the Tencent Cloud API credentials. Required.
	SecretID  string
	SecretKey string

	// AppID is the SMS application ID (SmsSdkAppId). Required.
	AppID string

	// SignName is the SMS signature registered in the Tencent console.
	SignName string

	// TemplateID is the SMS template ID. Used as default.
	TemplateID string

	// Region is the Tencent Cloud region. Default: "ap-guangzhou".
	Region string

	// HTTPTimeout sets the HTTP client timeout. Default: 10 seconds.
	HTTPTimeout time.Duration
}

// SmsTencentSender implements SmsSender using the Tencent Cloud SMS API v3.
type SmsTencentSender struct {
	cfg    SmsTencentConfig
	client *http.Client
}

// NewSmsTencentSender creates a Tencent Cloud SMS Sender.
func NewSmsTencentSender(cfg SmsTencentConfig) *SmsTencentSender {
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &SmsTencentSender{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

// Send delivers an SMS via the Tencent Cloud SMS API.
func (s *SmsTencentSender) Send(ctx context.Context, msg *SmsMessage) error {
	signName := msg.SignName
	if signName == "" {
		signName = s.cfg.SignName
	}
	templateID := msg.TemplateCode
	if templateID == "" {
		templateID = s.cfg.TemplateID
	}

	var templateParamSet []string
	if len(msg.Params) > 0 {
		keys := make([]string, 0, len(msg.Params))
		for k := range msg.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			templateParamSet = append(templateParamSet, msg.Params[k])
		}
	}

	payload := map[string]any{
		"SmsSdkAppId":      s.cfg.AppID,
		"SignName":         signName,
		"TemplateId":       templateID,
		"TemplateParamSet": templateParamSet,
		"PhoneNumberSet":   []string{msg.To},
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tcSMSEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sms/tencent: build request: %w", err)
	}

	now := time.Now().UTC()
	ts := fmt.Sprintf("%d", now.Unix())
	date := now.Format("2006-01-02")

	authHeader := smsTencentTC3Sign(s.cfg.SecretID, s.cfg.SecretKey, ts, date, body)

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-TC-Action", tcSMSAction)
	req.Header.Set("X-TC-Version", tcSMSVersion)
	req.Header.Set("X-TC-Timestamp", ts)
	req.Header.Set("X-TC-Region", s.cfg.Region)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sms/tencent: send: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Response struct {
			SendStatusSet []struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"SendStatusSet"`
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("sms/tencent: parse response: %w", err)
	}
	if e := result.Response.Error; e != nil {
		return fmt.Errorf("sms/tencent: API error %s: %s", e.Code, e.Message)
	}
	for _, st := range result.Response.SendStatusSet {
		if st.Code != "Ok" {
			return fmt.Errorf("sms/tencent: send failed %s: %s", st.Code, st.Message)
		}
	}
	return nil
}

// Verify SmsTencentSender implements SmsSender at compile time.
var _ SmsSender = (*SmsTencentSender)(nil)

func smsTencentTC3Sign(secretID, secretKey, timestamp, date string, payload []byte) string {
	bodyHash := smsTencentSHA256Hex(payload)
	canonicalReq := strings.Join([]string{
		"POST",
		"/",
		"",
		"content-type:application/json; charset=utf-8\nhost:sms.tencentcloudapi.com\n",
		"content-type;host",
		bodyHash,
	}, "\n")

	credScope := date + "/" + tcSMSService + "/tc3_request"
	strToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		timestamp,
		credScope,
		smsTencentSHA256Hex([]byte(canonicalReq)),
	}, "\n")

	signingKey := smsTencentTC3DeriveKey(secretKey, date)

	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(strToSign))
	sig := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=content-type;host, Signature=%s",
		secretID, credScope, sig)
}

func smsTencentTC3DeriveKey(secretKey, date string) []byte {
	h := func(key, data []byte) []byte {
		mac := hmac.New(sha256.New, key)
		mac.Write(data)
		return mac.Sum(nil)
	}
	secretDate := h([]byte("TC3"+secretKey), []byte(date))
	secretService := h(secretDate, []byte(tcSMSService))
	secretSigning := h(secretService, []byte("tc3_request"))
	return secretSigning
}

func smsTencentSHA256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
