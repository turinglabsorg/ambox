package forward

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Notification struct {
	Type       string `json:"type"`
	EmailID    string `json:"email_id"`
	From       string `json:"from"`
	ReceivedAt string `json:"received_at"`
	Folder     string `json:"folder"`
}

type Forwarder struct {
	httpClient *http.Client
}

func New() *Forwarder {
	return &Forwarder{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (f *Forwarder) NotifyAsync(webhookURL, webhookSecret string, notif *Notification) {
	go f.notifyWithRetry(webhookURL, webhookSecret, notif)
}

func (f *Forwarder) notifyWithRetry(webhookURL, webhookSecret string, notif *Notification) {
	delays := []time.Duration{0, 1 * time.Second, 5 * time.Second, 25 * time.Second}

	for i, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		if err := f.send(webhookURL, webhookSecret, notif); err != nil {
			log.Printf("webhook attempt %d/%d failed for %s: %v", i+1, len(delays), webhookURL, err)
			continue
		}
		return
	}
	log.Printf("webhook delivery failed after %d attempts for %s", len(delays), webhookURL)
}

func (f *Forwarder) send(webhookURL, webhookSecret string, notif *Notification) error {
	body, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if webhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Ambox-Signature", "sha256="+sig)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
