package resend

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxTimestampAge = 5 * time.Minute

type InboundWebhookData struct {
	EmailID string   `json:"email_id"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
}

type InboundWebhookPayload struct {
	Type string             `json:"type"`
	Data InboundWebhookData `json:"data"`
}

type InboundEmailContent struct {
	HTML        string              `json:"html"`
	Text        string              `json:"text"`
	Attachments []InboundAttachment `json:"attachments,omitempty"`
}

type InboundAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // base64
}

func VerifyWebhookSignature(secret string, headers http.Header, body []byte) error {
	msgID := headers.Get("svix-id")
	timestamp := headers.Get("svix-timestamp")
	signature := headers.Get("svix-signature")

	if msgID == "" || timestamp == "" || signature == "" {
		return fmt.Errorf("missing svix headers")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if math.Abs(float64(time.Now().Unix()-ts)) > maxTimestampAge.Seconds() {
		return fmt.Errorf("timestamp too old")
	}

	toSign := fmt.Sprintf("%s.%s.%s", msgID, timestamp, string(body))

	secretClean := strings.TrimPrefix(secret, "whsec_")
	secretBytes, err := base64.StdEncoding.DecodeString(secretClean)
	if err != nil {
		return fmt.Errorf("decode secret: %w", err)
	}

	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(toSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, sig := range strings.Split(signature, " ") {
		parts := strings.SplitN(sig, ",", 2)
		if len(parts) == 2 && parts[0] == "v1" {
			if hmac.Equal([]byte(parts[1]), []byte(expected)) {
				return nil
			}
		}
	}

	return fmt.Errorf("invalid signature")
}

func ParseInboundWebhook(body []byte) (*InboundWebhookPayload, error) {
	var payload InboundWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook payload: %w", err)
	}
	return &payload, nil
}
