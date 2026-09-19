//go:build integration

// Package s3test boots a disposable MinIO for integration tests, so the
// S3 adapter and glossa-edge are tested against a real S3 API.
package s3test

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore/s3store"
)

const (
	image    = "quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z" // Docker Hub no longer serves MinIO
	user     = "glossa-test"
	password = "glossa-test-secret"
)

// Env is a running MinIO.
type Env struct {
	// Endpoint is host:port (plain HTTP).
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string

	container *tcminio.MinioContainer
}

// Start boots MinIO.
func Start(ctx context.Context) (*Env, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	ctr, err := tcminio.Run(ctx, image, tcminio.WithUsername(user), tcminio.WithPassword(password))
	if err != nil {
		return nil, fmt.Errorf("s3test: start minio: %w", err)
	}
	endpoint, err := ctr.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return nil, fmt.Errorf("s3test: endpoint: %w", err)
	}
	return &Env{Endpoint: endpoint, AccessKeyID: user, SecretAccessKey: password, container: ctr}, nil
}

// Config returns the adapter configuration for bucket.
func (e *Env) Config(bucket string) s3store.Config {
	return s3store.Config{
		Endpoint: e.Endpoint, Insecure: true, Region: "us-east-1", Bucket: bucket, PathStyle: true,
		AccessKeyID: e.AccessKeyID, SecretAccessKey: e.SecretAccessKey, Timeout: 10 * time.Second,
	}
}

// Store returns an adapter on bucket, creating the bucket.
func (e *Env) Store(ctx context.Context, bucket string) (*s3store.Store, error) {
	s, err := s3store.New(e.Config(bucket))
	if err != nil {
		return nil, err
	}
	return s, s.EnsureBucket(ctx)
}

// Close stops MinIO.
func (e *Env) Close() {
	if e.container != nil {
		_ = testcontainers.TerminateContainer(e.container)
	}
}
