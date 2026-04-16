package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://api.resend.com",
	}
}

type SendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	ReplyTo string   `json:"reply_to,omitempty"`
}

type SendResponse struct {
	ID string `json:"id"`
}

func (c *Client) SendEmail(ctx context.Context, req *SendRequest) (*SendResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resend API error %d: %s", resp.StatusCode, string(respBody))
	}

	var sendResp SendResponse
	if err := json.Unmarshal(respBody, &sendResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &sendResp, nil
}

func (c *Client) GetInboundEmail(ctx context.Context, emailID string) (*InboundEmailContent, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/emails/receiving/"+emailID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch inbound email: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resend API error %d: %s", resp.StatusCode, string(respBody))
	}

	var content InboundEmailContent
	if err := json.Unmarshal(respBody, &content); err != nil {
		return nil, fmt.Errorf("decode inbound email: %w", err)
	}

	return &content, nil
}

type AttachmentMeta struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
}

type AttachmentListResponse struct {
	Data []AttachmentMeta `json:"data"`
}

func (c *Client) ListInboundAttachments(ctx context.Context, emailID string) ([]AttachmentMeta, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/emails/receiving/"+emailID+"/attachments", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resend API error %d: %s", resp.StatusCode, string(respBody))
	}

	var listResp AttachmentListResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("decode attachments: %w", err)
	}

	return listResp.Data, nil
}

func (c *Client) DownloadAttachment(ctx context.Context, downloadURL string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
