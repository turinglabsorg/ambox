package crypto

import (
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if len(kp.PublicKeyPEM) == 0 {
		t.Fatal("empty public key")
	}
	if len(kp.PrivateKeyPEM) == 0 {
		t.Fatal("empty private key")
	}

	pub, err := ParsePublicKey(kp.PublicKeyPEM)
	if err != nil {
		t.Fatalf("ParsePublicKey: %v", err)
	}
	if pub.Size() != RSAKeySize/8 {
		t.Fatalf("expected key size %d, got %d", RSAKeySize/8, pub.Size())
	}

	priv, err := ParsePrivateKey(kp.PrivateKeyPEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	if priv.Size() != RSAKeySize/8 {
		t.Fatalf("expected key size %d, got %d", RSAKeySize/8, priv.Size())
	}
}

func TestEncryptDecryptEmail(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	pub, _ := ParsePublicKey(kp.PublicKeyPEM)
	priv, _ := ParsePrivateKey(kp.PrivateKeyPEM)

	subject := "Test Subject"
	body := `{"html":"<p>Hello</p>","text":"Hello"}`

	enc, err := EncryptEmail(pub, subject, body)
	if err != nil {
		t.Fatalf("EncryptEmail: %v", err)
	}

	if enc.SubjectEncrypted == "" || enc.BodyEncrypted == "" || enc.WrappedKey == "" {
		t.Fatal("encrypted fields should not be empty")
	}

	decSubject, decBody, err := DecryptEmail(priv, enc)
	if err != nil {
		t.Fatalf("DecryptEmail: %v", err)
	}

	if decSubject != subject {
		t.Fatalf("subject mismatch: got %q, want %q", decSubject, subject)
	}
	if decBody != body {
		t.Fatalf("body mismatch: got %q, want %q", decBody, body)
	}
}

func TestEncryptDecryptAttachment(t *testing.T) {
	kp, _ := GenerateKeyPair()
	pub, _ := ParsePublicKey(kp.PublicKeyPEM)
	priv, _ := ParsePrivateKey(kp.PrivateKeyPEM)

	data := []byte("this is a test PDF content")

	enc, err := EncryptAttachment(pub, data, 3)
	if err != nil {
		t.Fatalf("EncryptAttachment: %v", err)
	}

	dec, err := DecryptAttachment(priv, enc)
	if err != nil {
		t.Fatalf("DecryptAttachment: %v", err)
	}

	if string(dec) != string(data) {
		t.Fatalf("attachment mismatch: got %q, want %q", dec, data)
	}
}

func TestEncryptEmptyBody(t *testing.T) {
	kp, _ := GenerateKeyPair()
	pub, _ := ParsePublicKey(kp.PublicKeyPEM)
	priv, _ := ParsePrivateKey(kp.PrivateKeyPEM)

	enc, err := EncryptEmail(pub, "", "")
	if err != nil {
		t.Fatalf("EncryptEmail empty: %v", err)
	}

	s, b, err := DecryptEmail(priv, enc)
	if err != nil {
		t.Fatalf("DecryptEmail empty: %v", err)
	}
	if s != "" || b != "" {
		t.Fatalf("expected empty, got %q %q", s, b)
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if len(key) != len("amk_live_")+64 {
		t.Fatalf("unexpected key length: %d", len(key))
	}
	prefix := APIKeyPrefix(key)
	if len(prefix) != len("amk_live_")+8 {
		t.Fatalf("unexpected prefix length: %d", len(prefix))
	}
}

func TestHashVerifyAPIKey(t *testing.T) {
	key := "amk_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678"

	hash, err := HashAPIKey(key)
	if err != nil {
		t.Fatalf("HashAPIKey: %v", err)
	}

	if !VerifyAPIKey(key, hash) {
		t.Fatal("VerifyAPIKey should return true for correct key")
	}

	if VerifyAPIKey("wrong_key", hash) {
		t.Fatal("VerifyAPIKey should return false for wrong key")
	}
}
