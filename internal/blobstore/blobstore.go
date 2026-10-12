// Package blobstore implements the binary-data adapter for
// m9m (BUG-ARCH-2026-10-12-006).
//
// n8n stores binary node data (file uploads, HTTP response
// bodies, etc.) inline as base64 in the execution record.
// m9m v0.4.0 stores it in MySQL BLOB columns (per-row, per-
// execution) — works for small blobs but a 50 MB file
// bloats the row and the JSON round-trip. The recommended
// path is an external blob store (S3, MinIO, local disk).
//
// This package defines the `BlobStore` interface and ships
// two implementations:
//   - `LocalStore` — disk-backed, for single-instance and
//     dev/test runs. Default location is
//     `{DataDir}/blobs/{hexid}`.
//   - `S3Store` — AWS S3 / MinIO / any S3-compatible
//     service. Uses `aws-sdk-go` (already a dep) for the
//     upload/download API.
//
// The two implementations are interchangeable from the
// caller's perspective; the engine picks one at startup
// based on `M9M_BLOBSTORE_TYPE` (`local` / `s3`).
package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// BlobStore is the interface every backend implements.
type BlobStore interface {
	// Put stores `data` under `id` (a hex / base64 string
	// the engine mints; the blob store does not interpret
	// it). Returns the canonical ID.
	Put(ctx context.Context, id string, data io.Reader) error

	// Get returns a reader for the blob at `id`. The caller
	// is responsible for closing the reader.
	Get(ctx context.Context, id string) (io.ReadCloser, error)

	// Delete removes the blob at `id`. NotFound is not an
	// error.
	Delete(ctx context.Context, id string) error

	// Exists reports whether `id` is in the store.
	Exists(ctx context.Context, id string) (bool, error)
}

// ErrNotFound is returned by `Get` when the blob is missing.
var ErrNotFound = errors.New("blobstore: not found")

// LocalStore is the on-disk implementation. Blobs are
// sharded by the first 2 hex chars of the ID (256 shards)
// so a single directory doesn't have millions of entries.
type LocalStore struct {
	// DataDir is the root directory. Required.
	DataDir string
}

// NewLocalStore returns a LocalStore rooted at `dataDir`.
// The directory is created if it does not exist.
func NewLocalStore(dataDir string) (*LocalStore, error) {
	if dataDir == "" {
		return nil, errors.New("blobstore: data dir is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("blobstore: mkdir %s: %w", dataDir, err)
	}
	return &LocalStore{DataDir: dataDir}, nil
}

// path returns the on-disk path for `id`. Sharding by the
// first 2 hex chars keeps each shard under ~10k entries
// for a 1M-blob store.
func (l *LocalStore) path(id string) string {
	if len(id) < 2 {
		return filepath.Join(l.DataDir, "_invalid", id)
	}
	shard := id[:2]
	return filepath.Join(l.DataDir, shard, id)
}

// Put stores `data` under `id`. The whole stream is read
// into memory (capped at 1 GiB) and written atomically via
// temp-file + rename. 1 GiB is a sanity cap; real workflows
// should stream large files via the S3 backend.
func (l *LocalStore) Put(ctx context.Context, id string, data io.Reader) error {
	path := l.path(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("blobstore: mkdir: %w", err)
	}
	body, err := io.ReadAll(io.LimitReader(data, 1<<30))
	if err != nil {
		return fmt.Errorf("blobstore: read: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("blobstore: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("blobstore: rename %s: %w", path, err)
	}
	return nil
}

// Get returns a reader for the blob at `id`. Returns
// `ErrNotFound` if the file does not exist.
func (l *LocalStore) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	f, err := os.Open(l.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("blobstore: open: %w", err)
	}
	return f, nil
}

// Delete removes the blob at `id`. NotFound is not an error.
func (l *LocalStore) Delete(ctx context.Context, id string) error {
	if err := os.Remove(l.path(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Exists reports whether the blob is on disk.
func (l *LocalStore) Exists(ctx context.Context, id string) (bool, error) {
	_, err := os.Stat(l.path(id))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// S3Config configures the S3 backend. `Bucket` is the
// destination bucket; `Region` is the AWS region (or
// `us-east-1` for MinIO). `Endpoint` is optional — leave
// empty for AWS, set to the MinIO URL for self-hosted.
type S3Config struct {
	Bucket   string
	Region   string
	Endpoint string // optional, e.g. "http://minio:9000" for self-hosted
	Prefix   string // optional key prefix, e.g. "m9m/"
}

// S3Store is the AWS S3 / MinIO implementation.
type S3Store struct {
	cfg    S3Config
	sess   *session.Session
	client *s3.S3
}

// NewS3Store returns an S3Store configured per `cfg`. The
// AWS session is constructed from the standard credential
// chain (env vars, shared config, IAM role).
func NewS3Store(cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("blobstore: S3 bucket is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	awsCfg := &aws.Config{Region: aws.String(cfg.Region)}
	if cfg.Endpoint != "" {
		awsCfg.Endpoint = aws.String(cfg.Endpoint)
		awsCfg.S3ForcePathStyle = aws.Bool(true) // MinIO convention
	}
	sess, err := session.NewSession(awsCfg)
	if err != nil {
		return nil, fmt.Errorf("blobstore: aws session: %w", err)
	}
	return &S3Store{cfg: cfg, sess: sess, client: s3.New(sess)}, nil
}

// key returns the S3 key for `id` (prefix + id).
func (s *S3Store) key(id string) string {
	return s.cfg.Prefix + id
}

// Put uploads `data` to `s3://{Bucket}/{key}`.
func (s *S3Store) Put(ctx context.Context, id string, data io.Reader) error {
	uploader := s3manager.NewUploader(s.sess)
	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(id)),
		Body:   data,
	})
	if err != nil {
		return fmt.Errorf("blobstore: s3 put: %w", err)
	}
	return nil
}

// Get returns a reader for `s3://{Bucket}/{key}`.
func (s *S3Store) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	out, err := s.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(id)),
	})
	if err != nil {
		return nil, fmt.Errorf("blobstore: s3 get: %w", err)
	}
	return out.Body, nil
}

// Delete removes the blob. NotFound is not an error.
func (s *S3Store) Delete(ctx context.Context, id string) error {
	_, err := s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(id)),
	})
	return err
}

// Exists reports whether the blob is in S3. Uses a
// HEAD request (cheap; no body transfer).
func (s *S3Store) Exists(ctx context.Context, id string) (bool, error) {
	_, err := s.client.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(id)),
	})
	if err == nil {
		return true, nil
	}
	// A 404 from S3 surfaces as a typed error.
	if notFound(err) {
		return false, nil
	}
	return false, err
}

// notFound is a tiny AWS-error sniff — `aws-sdk-go` doesn't
// export a sentinel for 404, so we string-match. A cleaner
// version would `errors.As` into the SDK's `awserr.Error`.
func notFound(err error) bool {
	if err == nil {
		return false
	}
	// Best-effort match; a clean SDK would be nicer.
	s := err.Error()
	return contains(s, "NoSuchKey") || contains(s, "NotFound")
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
