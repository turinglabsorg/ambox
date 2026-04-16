package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

type EncryptedPayload struct {
	Ciphertext string // base64
	WrappedKey string // base64
	NonceIndex int    // deterministic nonce index used
}

type EncryptedEmail struct {
	SubjectEncrypted string // base64 AES-GCM ciphertext
	BodyEncrypted    string // base64 AES-GCM ciphertext
	WrappedKey       string // base64 RSA-OAEP wrapped AES key
}

func nonceFromIndex(index int) []byte {
	nonce := make([]byte, 12)
	nonce[11] = byte(index)
	return nonce
}

func EncryptEmail(publicKey *rsa.PublicKey, subject, body string) (*EncryptedEmail, error) {
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("generate aes key: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	subjectCt := gcm.Seal(nil, nonceFromIndex(1), []byte(subject), nil)
	bodyCt := gcm.Seal(nil, nonceFromIndex(2), []byte(body), nil)

	wrappedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("wrap aes key: %w", err)
	}

	return &EncryptedEmail{
		SubjectEncrypted: base64.StdEncoding.EncodeToString(subjectCt),
		BodyEncrypted:    base64.StdEncoding.EncodeToString(bodyCt),
		WrappedKey:       base64.StdEncoding.EncodeToString(wrappedKey),
	}, nil
}

func DecryptEmail(privateKey *rsa.PrivateKey, enc *EncryptedEmail) (subject, body string, err error) {
	wrappedKey, err := base64.StdEncoding.DecodeString(enc.WrappedKey)
	if err != nil {
		return "", "", fmt.Errorf("decode wrapped key: %w", err)
	}
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, wrappedKey, nil)
	if err != nil {
		return "", "", fmt.Errorf("unwrap aes key: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", "", fmt.Errorf("create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("create gcm: %w", err)
	}

	subjectCt, err := base64.StdEncoding.DecodeString(enc.SubjectEncrypted)
	if err != nil {
		return "", "", fmt.Errorf("decode subject: %w", err)
	}
	subjectBytes, err := gcm.Open(nil, nonceFromIndex(1), subjectCt, nil)
	if err != nil {
		return "", "", fmt.Errorf("decrypt subject: %w", err)
	}

	bodyCt, err := base64.StdEncoding.DecodeString(enc.BodyEncrypted)
	if err != nil {
		return "", "", fmt.Errorf("decode body: %w", err)
	}
	bodyBytes, err := gcm.Open(nil, nonceFromIndex(2), bodyCt, nil)
	if err != nil {
		return "", "", fmt.Errorf("decrypt body: %w", err)
	}

	return string(subjectBytes), string(bodyBytes), nil
}

func EncryptAttachment(publicKey *rsa.PublicKey, data []byte, nonceIndex int) (*EncryptedPayload, error) {
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("generate aes key: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	ct := gcm.Seal(nil, nonceFromIndex(nonceIndex), data, nil)

	wrappedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("wrap aes key: %w", err)
	}

	return &EncryptedPayload{
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
		WrappedKey: base64.StdEncoding.EncodeToString(wrappedKey),
		NonceIndex: nonceIndex,
	}, nil
}

func DecryptAttachment(privateKey *rsa.PrivateKey, payload *EncryptedPayload) ([]byte, error) {
	wrappedKey, err := base64.StdEncoding.DecodeString(payload.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("decode wrapped key: %w", err)
	}
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, wrappedKey, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap aes key: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	ct, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	return gcm.Open(nil, nonceFromIndex(payload.NonceIndex), ct, nil)
}
