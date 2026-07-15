package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/turinglabs/ambox/internal/crypto"
	"github.com/turinglabs/ambox/internal/middleware"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/store"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fakeEmailStore struct {
	inserted *store.Email
}

func (s *fakeEmailStore) CreateAgent(context.Context, *store.Agent) error { return nil }
func (s *fakeEmailStore) GetAgentByEmail(context.Context, string) (*store.Agent, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeEmailStore) UpdateAgent(context.Context, string, bson.M) error { return nil }
func (s *fakeEmailStore) InsertEmail(_ context.Context, email *store.Email) error {
	s.inserted = email
	return nil
}
func (s *fakeEmailStore) ListEmails(context.Context, store.InboxQuery) ([]store.Email, error) {
	return nil, nil
}
func (s *fakeEmailStore) GetEmail(context.Context, string, string) (*store.Email, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeEmailStore) DeleteEmail(context.Context, string, string) error { return nil }
func (s *fakeEmailStore) MoveEmail(context.Context, string, string, string) error {
	return nil
}

type fakeEmailProvider struct {
	sent    *resend.SendRequest
	sendErr error
}

func (p *fakeEmailProvider) SendEmail(_ context.Context, req *resend.SendRequest) (*resend.SendResponse, error) {
	p.sent = req
	if p.sendErr != nil {
		return nil, p.sendErr
	}
	return &resend.SendResponse{ID: "resend_test"}, nil
}
func (p *fakeEmailProvider) GetInboundEmail(context.Context, string) (*resend.InboundEmailContent, error) {
	return nil, errors.New("not implemented")
}
func (p *fakeEmailProvider) ListInboundAttachments(context.Context, string) ([]resend.AttachmentMeta, error) {
	return nil, errors.New("not implemented")
}
func (p *fakeEmailProvider) DownloadAttachment(context.Context, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

type fakeAttachmentStorage struct {
	files   map[string][]byte
	deleted []string
}

func newFakeAttachmentStorage() *fakeAttachmentStorage {
	return &fakeAttachmentStorage{files: map[string][]byte{}}
}

func (s *fakeAttachmentStorage) Upload(_ context.Context, path string, data []byte, _ string) error {
	s.files[path] = append([]byte(nil), data...)
	return nil
}
func (s *fakeAttachmentStorage) Download(_ context.Context, path string) ([]byte, error) {
	return s.files[path], nil
}
func (s *fakeAttachmentStorage) Delete(_ context.Context, path string) error {
	delete(s.files, path)
	s.deleted = append(s.deleted, path)
	return nil
}

func testAgent(t *testing.T) (*store.Agent, *rsa.PrivateKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return &store.Agent{
		ID:           "test-agent",
		Email:        "test-agent@ambox.dev",
		DisplayName:  "Test Agent",
		PublicKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})),
	}, privateKey
}

func sendRequest(t *testing.T, h *Handler, agent *store.Agent, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/send", strings.NewReader(string(payload)))
	ctx := context.WithValue(req.Context(), middleware.AgentContextKey, agent)
	recorder := httptest.NewRecorder()
	h.Send(recorder, req.WithContext(ctx))
	return recorder
}

func TestSendDeliversAndStoresEncryptedAttachment(t *testing.T) {
	agent, privateKey := testAgent(t)
	emailStore := &fakeEmailStore{}
	provider := &fakeEmailProvider{}
	storage := newFakeAttachmentStorage()
	h := New(emailStore, provider, nil, nil, storage, nil)
	attachmentData := []byte("city,code\nIspica,123456\n")

	response := sendRequest(t, h, agent, SendRequest{
		To:       []string{"recipient@example.com"},
		Subject:  "Mobility credentials",
		BodyText: "Attached.",
		Attachments: []SendAttachmentRequest{{
			Filename:    "credentials.csv",
			ContentType: "text/csv",
			Content:     base64.StdEncoding.EncodeToString(attachmentData),
		}},
	})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if provider.sent == nil || len(provider.sent.Attachments) != 1 {
		t.Fatalf("provider attachments = %#v", provider.sent)
	}
	providerData, err := base64.StdEncoding.DecodeString(provider.sent.Attachments[0].Content)
	if err != nil {
		t.Fatalf("decode provider content: %v", err)
	}
	if string(providerData) != string(attachmentData) {
		t.Fatalf("provider attachment = %q, want %q", providerData, attachmentData)
	}
	if emailStore.inserted == nil || len(emailStore.inserted.Attachments) != 1 {
		t.Fatalf("stored email attachments = %#v", emailStore.inserted)
	}

	storedAttachment := emailStore.inserted.Attachments[0]
	ciphertext := storage.files[storedAttachment.GCSPath]
	decrypted, err := crypto.DecryptAttachment(privateKey, &crypto.EncryptedPayload{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		WrappedKey: storedAttachment.WrappedKey,
		NonceIndex: storedAttachment.NonceIndex,
	})
	if err != nil {
		t.Fatalf("decrypt stored attachment: %v", err)
	}
	if string(decrypted) != string(attachmentData) {
		t.Fatalf("stored attachment = %q, want %q", decrypted, attachmentData)
	}
}

func TestSendRejectsAttachmentsWithoutStorage(t *testing.T) {
	agent, _ := testAgent(t)
	provider := &fakeEmailProvider{}
	h := New(&fakeEmailStore{}, provider, nil, nil, nil, nil)

	response := sendRequest(t, h, agent, SendRequest{
		To:       []string{"recipient@example.com"},
		Subject:  "Attachment",
		BodyText: "Attached.",
		Attachments: []SendAttachmentRequest{{
			Filename: "file.txt",
			Content:  base64.StdEncoding.EncodeToString([]byte("hello")),
		}},
	})

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if provider.sent != nil {
		t.Fatal("provider should not be called")
	}
}

func TestSendCleansStoredAttachmentsWhenProviderFails(t *testing.T) {
	agent, _ := testAgent(t)
	provider := &fakeEmailProvider{sendErr: errors.New("provider unavailable")}
	storage := newFakeAttachmentStorage()
	h := New(&fakeEmailStore{}, provider, nil, nil, storage, nil)

	response := sendRequest(t, h, agent, SendRequest{
		To:       []string{"recipient@example.com"},
		Subject:  "Attachment",
		BodyText: "Attached.",
		Attachments: []SendAttachmentRequest{{
			Filename: "file.txt",
			Content:  base64.StdEncoding.EncodeToString([]byte("hello")),
		}},
	})

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(storage.files) != 0 || len(storage.deleted) != 1 {
		t.Fatalf("files = %d, deleted = %v", len(storage.files), storage.deleted)
	}
}

func TestPrepareOutgoingAttachmentsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		attachments []SendAttachmentRequest
	}{
		{
			name: "invalid base64",
			attachments: []SendAttachmentRequest{{
				Filename: "file.txt",
				Content:  "not-base64",
			}},
		},
		{
			name: "path in filename",
			attachments: []SendAttachmentRequest{{
				Filename: "private/file.txt",
				Content:  base64.StdEncoding.EncodeToString([]byte("hello")),
			}},
		},
		{
			name: "duplicate filename",
			attachments: []SendAttachmentRequest{
				{Filename: "file.txt", Content: base64.StdEncoding.EncodeToString([]byte("one"))},
				{Filename: "FILE.txt", Content: base64.StdEncoding.EncodeToString([]byte("two"))},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := prepareOutgoingAttachments(&SendRequest{Attachments: test.attachments})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
