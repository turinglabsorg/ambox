package store

import "time"

type Agent struct {
	ID            string    `bson:"_id" json:"id"`
	Email         string    `bson:"email" json:"email"`
	DisplayName   string    `bson:"display_name,omitempty" json:"display_name,omitempty"`
	APIKeyHash    string    `bson:"api_key_hash" json:"-"`
	APIKeyPrefix  string    `bson:"api_key_prefix" json:"-"`
	PublicKeyPEM  string    `bson:"public_key_pem" json:"-"`
	WebhookURL    string    `bson:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	WebhookSecret string    `bson:"webhook_secret,omitempty" json:"-"`
	TTLSeconds    int64     `bson:"ttl_seconds" json:"ttl_seconds"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
}

type Attachment struct {
	Filename    string `bson:"filename" json:"filename"`
	ContentType string `bson:"content_type" json:"content_type"`
	SizeBytes   int64  `bson:"size_bytes" json:"size_bytes"`
	GCSPath     string `bson:"gcs_path" json:"gcs_path"`
	WrappedKey  string `bson:"wrapped_key" json:"wrapped_key"`
	NonceIndex  int    `bson:"nonce_index" json:"nonce_index"`
}

type Email struct {
	ID               string       `bson:"_id" json:"id"`
	AgentID          string       `bson:"agent_id" json:"agent_id"`
	Folder           string       `bson:"folder" json:"folder"`
	From             string       `bson:"from" json:"from"`
	To               []string     `bson:"to" json:"to"`
	SubjectEncrypted string       `bson:"subject_encrypted" json:"subject_encrypted"`
	BodyEncrypted    string       `bson:"body_encrypted" json:"body_encrypted"`
	WrappedKey       string       `bson:"wrapped_key" json:"wrapped_key"`
	ResendID         string       `bson:"resend_id,omitempty" json:"resend_id,omitempty"`
	Attachments      []Attachment `bson:"attachments,omitempty" json:"attachments,omitempty"`
	Classification   string       `bson:"classification,omitempty" json:"classification,omitempty"`
	ReceivedAt       time.Time    `bson:"received_at" json:"received_at"`
	ExpiresAt        *time.Time   `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	CreatedAt        time.Time    `bson:"created_at" json:"created_at"`
}
