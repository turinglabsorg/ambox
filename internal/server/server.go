package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/turinglabs/ambox/internal/classify"
	"github.com/turinglabs/ambox/internal/config"
	"github.com/turinglabs/ambox/internal/forward"
	"github.com/turinglabs/ambox/internal/handler"
	"github.com/turinglabs/ambox/internal/middleware"
	"github.com/turinglabs/ambox/internal/resend"
	"github.com/turinglabs/ambox/internal/storage"
	"github.com/turinglabs/ambox/internal/store"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func Run() {
	cfg := config.FromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatalf("connect to mongodb: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("ping mongodb: %v", err)
	}
	log.Println("connected to mongodb")

	db := mongoClient.Database(cfg.MongoDatabase)
	s := store.New(db)

	if err := s.EnsureIndexes(ctx); err != nil {
		log.Printf("warning: ensure indexes: %v (non-fatal, indexes may need manual creation)", err)
	}

	resendClient := resend.NewClient(cfg.ResendAPIKey)
	classifier := classify.New(cfg.OllamaBaseURL, cfg.OllamaAPIKey, cfg.OllamaModel)
	forwarder := forward.New()

	var gcsStorage *storage.GCS
	if cfg.GCSBucket != "" {
		gcsStorage, err = storage.NewGCS(ctx, cfg.GCSBucket)
		if err != nil {
			log.Fatalf("create gcs client: %v", err)
		}
		defer gcsStorage.Close()
		log.Printf("gcs bucket: %s", cfg.GCSBucket)
	}

	h := handler.New(s, resendClient, classifier, forwarder, gcsStorage, cfg)

	registerLimiter := middleware.NewRateLimiter(rate.Every(12*time.Minute), 5)
	agentLimiter := middleware.NewRateLimiter(rate.Limit(100.0/60.0), 10)
	maxBody := middleware.MaxBodySize(10 * 1024 * 1024)

	auth := middleware.Auth(s)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", h.Health)

	mux.Handle("POST /v1/register", registerLimiter.ByIP(maxBody(http.HandlerFunc(h.Register))))

	mux.Handle("POST /v1/send", auth(agentLimiter.ByAgent(maxBody(http.HandlerFunc(h.Send)))))
	mux.Handle("GET /v1/inbox", auth(agentLimiter.ByAgent(http.HandlerFunc(h.Inbox))))
	mux.Handle("DELETE /v1/emails/{id}", auth(agentLimiter.ByAgent(http.HandlerFunc(h.DeleteEmail))))
	mux.Handle("PUT /v1/emails/{id}/move", auth(agentLimiter.ByAgent(maxBody(http.HandlerFunc(h.MoveEmail)))))
	mux.Handle("GET /v1/emails/{id}/attachments/{filename}", auth(agentLimiter.ByAgent(http.HandlerFunc(h.DownloadAttachment))))
	mux.Handle("PUT /v1/webhook", auth(agentLimiter.ByAgent(maxBody(http.HandlerFunc(h.ConfigureWebhook)))))
	mux.Handle("PUT /v1/settings", auth(agentLimiter.ByAgent(maxBody(http.HandlerFunc(h.Settings)))))

	mux.Handle("POST /v1/inbound", maxBody(http.HandlerFunc(h.Inbound)))

	// Serve og.png
	mux.HandleFunc("GET /og.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, "/web/og.png")
	})

	// Serve skill.md
	mux.HandleFunc("GET /skill.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		http.ServeFile(w, r, "/web/skill.md")
	})

	// Landing page
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "/web/index.html")
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("ambox server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown: %v", err)
	}
	log.Println("server stopped")
}
