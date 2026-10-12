package blobstore

// Tests for BUG-ARCH-2026-10-12-006. Hermetic — the S3
// backend is exercised via the `BlobStore` interface
// against a fake; the local backend uses `t.TempDir()`.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeStore is an in-memory BlobStore used to verify the
// S3 dispatch path without standing up MinIO.
type fakeStore struct {
	data map[string][]byte
}

func newFake() *fakeStore { return &fakeStore{data: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, id string, data io.Reader) error {
	body, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	f.data[id] = body
	return nil
}
func (f *fakeStore) Get(_ context.Context, id string) (io.ReadCloser, error) {
	b, ok := f.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (f *fakeStore) Delete(_ context.Context, id string) error {
	delete(f.data, id)
	return nil
}
func (f *fakeStore) Exists(_ context.Context, id string) (bool, error) {
	_, ok := f.data[id]
	return ok, nil
}

func TestBlobStoreInterface_HappyPath(t *testing.T) {
	var s BlobStore = newFake()
	if err := s.Put(context.Background(), "abc123", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, err := s.Get(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "hello" {
		t.Errorf("body = %q, want hello", body)
	}
	ok, err := s.Exists(context.Background(), "abc123")
	if err != nil || !ok {
		t.Errorf("Exists = %v, %v", ok, err)
	}
	if err := s.Delete(context.Background(), "abc123"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	ok, _ = s.Exists(context.Background(), "abc123")
	if ok {
		t.Errorf("Exists after delete = true")
	}
}

func TestBlobStore_GetNotFound(t *testing.T) {
	s := newFake()
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
	}
}

func TestLocalStore_PutGetDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "abcdef", strings.NewReader("data-1")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	r, err := s.Get(ctx, "abcdef")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	body, _ := io.ReadAll(r)
	r.Close()
	if string(body) != "data-1" {
		t.Errorf("body = %q", body)
	}
	ok, _ := s.Exists(ctx, "abcdef")
	if !ok {
		t.Error("Exists = false after Put")
	}
	if err := s.Delete(ctx, "abcdef"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	ok, _ = s.Exists(ctx, "abcdef")
	if ok {
		t.Error("Exists = true after Delete")
	}
}

func TestLocalStore_Sharding(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStore(dir)
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	// 256 shards — the first 2 hex chars.
	if path := s.path("ff00"); path != dir+"/ff/ff00" {
		t.Errorf("path = %q", path)
	}
	if path := s.path("00ab"); path != dir+"/00/00ab" {
		t.Errorf("path = %q", path)
	}
}

func TestLocalStore_EmptyDataDirRejected(t *testing.T) {
	if _, err := NewLocalStore(""); err == nil {
		t.Fatal("expected error on empty data dir")
	}
}

func TestLocalStore_NotFoundReturnsErrNotFound(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalStore(dir)
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestNewS3Store_ValidatesBucket(t *testing.T) {
	if _, err := NewS3Store(S3Config{}); err == nil {
		t.Fatal("expected error on empty bucket")
	}
}

func TestS3Store_KeyWithPrefix(t *testing.T) {
	s, err := NewS3Store(S3Config{Bucket: "b", Region: "us-east-1", Prefix: "m9m/"})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	if s.key("abc") != "m9m/abc" {
		t.Errorf("key = %q, want m9m/abc", s.key("abc"))
	}
	// No prefix.
	s2, _ := NewS3Store(S3Config{Bucket: "b", Region: "us-east-1"})
	if s2.key("abc") != "abc" {
		t.Errorf("key (no prefix) = %q, want abc", s2.key("abc"))
	}
}

func TestNotFound(t *testing.T) {
	if notFound(nil) {
		t.Error("notFound(nil) = true")
	}
	if !notFound(errors.New("NoSuchKey: not here")) {
		t.Error("notFound(NoSuchKey) = false")
	}
	if !notFound(errors.New("NotFound: 404")) {
		t.Error("notFound(NotFound) = false")
	}
	if notFound(errors.New("AccessDenied")) {
		t.Error("notFound(AccessDenied) = true")
	}
}
