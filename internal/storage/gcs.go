package storage

import (
	"context"
	"fmt"
	"io"

	gcs "cloud.google.com/go/storage"
)

type GCS struct {
	client *gcs.Client
	bucket string
}

func NewGCS(ctx context.Context, bucket string) (*GCS, error) {
	client, err := gcs.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gcs client: %w", err)
	}
	return &GCS{client: client, bucket: bucket}, nil
}

func (g *GCS) Upload(ctx context.Context, path string, data []byte, contentType string) error {
	w := g.client.Bucket(g.bucket).Object(path).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := w.Write(data); err != nil {
		w.Close()
		return fmt.Errorf("write to gcs: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close gcs writer: %w", err)
	}
	return nil
}

func (g *GCS) Download(ctx context.Context, path string) ([]byte, error) {
	r, err := g.client.Bucket(g.bucket).Object(path).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("open gcs reader: %w", err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read from gcs: %w", err)
	}
	return data, nil
}

func (g *GCS) Delete(ctx context.Context, path string) error {
	if err := g.client.Bucket(g.bucket).Object(path).Delete(ctx); err != nil {
		return fmt.Errorf("delete from gcs: %w", err)
	}
	return nil
}

func (g *GCS) Close() error {
	return g.client.Close()
}
