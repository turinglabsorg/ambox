package config

import "os"

type Config struct {
	Port                string
	MongoURI            string
	MongoDatabase       string
	ResendAPIKey        string
	ResendWebhookSecret string
	GCSBucket           string
	OllamaBaseURL       string
	OllamaAPIKey        string
	OllamaModel         string
	EmailDomain         string
}

func FromEnv() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	emailDomain := os.Getenv("EMAIL_DOMAIN")
	if emailDomain == "" {
		emailDomain = "ambox.dev"
	}
	ollamaURL := os.Getenv("OLLAMA_BASE_URL")
	if ollamaURL == "" {
		ollamaURL = "https://ollama.com/v1"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "qwen2.5:7b"
	}

	return &Config{
		Port:                port,
		MongoURI:            os.Getenv("MONGODB_URI"),
		MongoDatabase:       os.Getenv("MONGODB_DATABASE"),
		ResendAPIKey:        os.Getenv("RESEND_API_KEY"),
		ResendWebhookSecret: os.Getenv("RESEND_WEBHOOK_SECRET"),
		GCSBucket:           os.Getenv("GCS_BUCKET"),
		OllamaBaseURL:       ollamaURL,
		OllamaAPIKey:        os.Getenv("OLLAMA_API_KEY"),
		OllamaModel:         ollamaModel,
		EmailDomain:         emailDomain,
	}
}
