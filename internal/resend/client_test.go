package resend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendEmailIncludesAttachments(t *testing.T) {
	var received SendRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resend_attachment_test"}`))
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL
	response, err := client.SendEmail(context.Background(), &SendRequest{
		From:    "agent@ambox.dev",
		To:      []string{"recipient@example.com"},
		Subject: "Attachment test",
		Text:    "Attached.",
		Attachments: []Attachment{{
			Filename: "credentials.csv",
			Content:  "Y2l0eSxjb2RlCg==",
		}},
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}
	if response.ID != "resend_attachment_test" {
		t.Fatalf("response id = %q", response.ID)
	}
	if len(received.Attachments) != 1 {
		t.Fatalf("attachments = %#v", received.Attachments)
	}
	if received.Attachments[0].Filename != "credentials.csv" || received.Attachments[0].Content != "Y2l0eSxjb2RlCg==" {
		t.Fatalf("attachment = %#v", received.Attachments[0])
	}
}
