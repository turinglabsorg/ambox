package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var validFolders = map[string]bool{
	"inbox":         true,
	"important":     true,
	"transactional": true,
	"notification":  true,
	"spam":          true,
}

type Classifier struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func New(baseURL, apiKey, model string) *Classifier {
	if model == "" {
		model = "qwen2.5:7b"
	}
	if baseURL == "" {
		baseURL = "https://ollama.com/v1"
	}
	return &Classifier{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Classifier) Classify(ctx context.Context, from, subject, bodyPreview string) string {
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500]
	}

	prompt := fmt.Sprintf(
		"Classify this email into exactly one category. Reply with a single word, nothing else.\n\nCategories: inbox, important, transactional, notification, spam\n\nFrom: %s\nSubject: %s\nBody preview: %s",
		from, subject, bodyPreview,
	)

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "inbox"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "inbox"
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "inbox"
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "inbox"
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "inbox"
	}

	if len(chatResp.Choices) == 0 {
		return "inbox"
	}

	folder := strings.TrimSpace(strings.ToLower(chatResp.Choices[0].Message.Content))
	if !validFolders[folder] {
		return "inbox"
	}
	return folder
}
