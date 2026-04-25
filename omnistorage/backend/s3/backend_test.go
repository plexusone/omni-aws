package s3

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	omnistorage "github.com/plexusone/omnistorage-core/object"
)

// Integration tests that require a real S3-compatible service.
// Set these environment variables to run integration tests:
//   - OMNISTORAGE_S3_TEST_BUCKET: bucket name
//   - OMNISTORAGE_S3_TEST_REGION: region (optional)
//   - OMNISTORAGE_S3_TEST_ENDPOINT: endpoint (optional, for MinIO/R2)
//   - AWS_ACCESS_KEY_ID: access key
//   - AWS_SECRET_ACCESS_KEY: secret key

func getTestBackend(t *testing.T) *Backend {
	bucket := os.Getenv("OMNISTORAGE_S3_TEST_BUCKET")
	if bucket == "" {
		t.Skip("OMNISTORAGE_S3_TEST_BUCKET not set, skipping integration test")
	}

	cfg := Config{
		Bucket:       bucket,
		Region:       os.Getenv("OMNISTORAGE_S3_TEST_REGION"),
		Endpoint:     os.Getenv("OMNISTORAGE_S3_TEST_ENDPOINT"),
		Prefix:       "omnistorage-test-" + time.Now().Format("20060102-150405"),
		UsePathStyle: os.Getenv("OMNISTORAGE_S3_USE_PATH_STYLE") == "true",
	}

	backend, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create S3 backend: %v", err)
	}

	return backend
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "empty bucket",
			config:  Config{},
			wantErr: true,
		},
		{
			name:    "valid config",
			config:  Config{Bucket: "my-bucket"},
			wantErr: false,
		},
		{
			name:    "valid config with region",
			config:  Config{Bucket: "my-bucket", Region: "us-east-1"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PartSize != 5*1024*1024 {
		t.Errorf("PartSize = %d, want %d", cfg.PartSize, 5*1024*1024)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("Concurrency = %d, want 5", cfg.Concurrency)
	}
}

func TestConfigFromMap(t *testing.T) {
	m := map[string]string{
		"bucket":            "my-bucket",
		"region":            "us-west-2",
		"endpoint":          "http://localhost:9000",
		"prefix":            "data/",
		"access_key_id":     "mykey",
		"secret_access_key": "mysecret",
		"use_path_style":    "true",
		"part_size":         "10485760",
		"concurrency":       "10",
	}

	cfg := ConfigFromMap(m)

	if cfg.Bucket != "my-bucket" {
		t.Errorf("Bucket = %q, want %q", cfg.Bucket, "my-bucket")
	}
	if cfg.Region != "us-west-2" {
		t.Errorf("Region = %q, want %q", cfg.Region, "us-west-2")
	}
	if cfg.Endpoint != "http://localhost:9000" {
		t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, "http://localhost:9000")
	}
	if cfg.Prefix != "data/" {
		t.Errorf("Prefix = %q, want %q", cfg.Prefix, "data/")
	}
	if cfg.AccessKeyID != "mykey" {
		t.Errorf("AccessKeyID = %q, want %q", cfg.AccessKeyID, "mykey")
	}
	if cfg.SecretAccessKey != "mysecret" {
		t.Errorf("SecretAccessKey = %q, want %q", cfg.SecretAccessKey, "mysecret")
	}
	if !cfg.UsePathStyle {
		t.Error("UsePathStyle = false, want true")
	}
	if cfg.PartSize != 10485760 {
		t.Errorf("PartSize = %d, want %d", cfg.PartSize, 10485760)
	}
	if cfg.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want %d", cfg.Concurrency, 10)
	}
}

func TestFeatures(t *testing.T) {
	// Create a mock backend just to test Features
	backend := &Backend{
		config: Config{Bucket: "test"},
	}

	features := backend.Features()

	if !features.Copy {
		t.Error("Features.Copy = false, want true")
	}
	if !features.Move {
		t.Error("Features.Move = false, want true")
	}
	if !features.Stat {
		t.Error("Features.Stat = false, want true")
	}
	if !features.RangeRead {
		t.Error("Features.RangeRead = false, want true")
	}
	if !features.ListPrefix {
		t.Error("Features.ListPrefix = false, want true")
	}
	if !features.SupportsHash(omnistorage.HashMD5) {
		t.Error("Features should support MD5 hash")
	}
}

func TestFullKey(t *testing.T) {
	tests := []struct {
		prefix   string
		path     string
		expected string
	}{
		{"", "file.txt", "file.txt"},
		{"data", "file.txt", "data/file.txt"},
		{"data/", "file.txt", "data/file.txt"},
		{"data", "sub/file.txt", "data/sub/file.txt"},
	}

	for _, tt := range tests {
		backend := &Backend{config: Config{Prefix: tt.prefix}}
		result := backend.fullKey(tt.path)
		if result != tt.expected {
			t.Errorf("fullKey(%q) with prefix %q = %q, want %q", tt.path, tt.prefix, result, tt.expected)
		}
	}
}

func TestExtendedBackendInterface(t *testing.T) {
	backend := &Backend{config: Config{Bucket: "test"}}

	// Verify backend implements ExtendedBackend
	var _ omnistorage.ExtendedBackend = backend

	// Test AsExtended helper
	ext, ok := omnistorage.AsExtended(backend)
	if !ok {
		t.Error("AsExtended returned false for S3 backend")
	}
	if ext == nil {
		t.Error("AsExtended returned nil for S3 backend")
	}
}

// Integration tests - only run when OMNISTORAGE_S3_TEST_BUCKET is set

func TestIntegrationWriteRead(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Write
	w, err := backend.NewWriter(ctx, "test.txt", omnistorage.WithContentType("text/plain"))
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	data := []byte("hello S3 world")
	if _, err := w.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Read
	r, err := backend.NewReader(ctx, "test.txt")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	readData, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	_ = r.Close()

	if string(readData) != string(data) {
		t.Errorf("Read data = %q, want %q", readData, data)
	}

	// Cleanup
	_ = backend.Delete(ctx, "test.txt")
}

func TestIntegrationExists(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Should not exist
	exists, err := backend.Exists(ctx, "nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("File should not exist")
	}

	// Create file
	w, _ := backend.NewWriter(ctx, "exists-test.txt")
	_, _ = w.Write([]byte("test"))
	_ = w.Close()

	// Should exist
	exists, err = backend.Exists(ctx, "exists-test.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("File should exist")
	}

	// Cleanup
	_ = backend.Delete(ctx, "exists-test.txt")
}

func TestIntegrationDelete(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Create file
	w, _ := backend.NewWriter(ctx, "delete-test.txt")
	_, _ = w.Write([]byte("test"))
	_ = w.Close()

	// Delete
	if err := backend.Delete(ctx, "delete-test.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not exist
	exists, _ := backend.Exists(ctx, "delete-test.txt")
	if exists {
		t.Error("File should not exist after delete")
	}
}

func TestIntegrationDeleteIdempotent(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Delete non-existent should not error
	if err := backend.Delete(ctx, "nonexistent-delete.txt"); err != nil {
		t.Errorf("Delete of non-existent file failed: %v", err)
	}
}

func TestIntegrationList(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Create some files
	files := []string{"list-a.txt", "list-b.txt", "list-sub/c.txt"}
	for _, f := range files {
		w, _ := backend.NewWriter(ctx, f)
		_, _ = w.Write([]byte("test"))
		_ = w.Close()
	}

	// List all
	paths, err := backend.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// Should have at least our test files
	found := 0
	for _, p := range paths {
		for _, f := range files {
			if p == f {
				found++
				break
			}
		}
	}

	if found != len(files) {
		t.Errorf("Found %d of %d test files in list", found, len(files))
	}

	// Cleanup
	for _, f := range files {
		_ = backend.Delete(ctx, f)
	}
}

func TestIntegrationStat(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Create file
	data := []byte("stat test data")
	w, _ := backend.NewWriter(ctx, "stat-test.txt", omnistorage.WithContentType("text/plain"))
	_, _ = w.Write(data)
	_ = w.Close()

	// Stat
	info, err := backend.Stat(ctx, "stat-test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Size() != int64(len(data)) {
		t.Errorf("Size = %d, want %d", info.Size(), len(data))
	}
	if info.ModTime().IsZero() {
		t.Error("ModTime is zero")
	}
	if info.ContentType() != "text/plain" {
		t.Errorf("ContentType = %q, want %q", info.ContentType(), "text/plain")
	}

	// Cleanup
	_ = backend.Delete(ctx, "stat-test.txt")
}

func TestIntegrationStatNotFound(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	_, err := backend.Stat(ctx, "nonexistent-stat.txt")
	if !omnistorage.IsNotFound(err) {
		t.Errorf("Stat error = %v, want ErrNotFound", err)
	}
}

func TestIntegrationCopy(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Create source
	data := []byte("copy test data")
	w, _ := backend.NewWriter(ctx, "copy-src.txt")
	_, _ = w.Write(data)
	_ = w.Close()

	// Copy
	if err := backend.Copy(ctx, "copy-src.txt", "copy-dst.txt"); err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// Verify destination
	r, err := backend.NewReader(ctx, "copy-dst.txt")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	readData, _ := io.ReadAll(r)
	_ = r.Close()

	if string(readData) != string(data) {
		t.Errorf("Copied data = %q, want %q", readData, data)
	}

	// Verify source still exists
	exists, _ := backend.Exists(ctx, "copy-src.txt")
	if !exists {
		t.Error("Source should still exist after copy")
	}

	// Cleanup
	_ = backend.Delete(ctx, "copy-src.txt")
	_ = backend.Delete(ctx, "copy-dst.txt")
}

func TestIntegrationMove(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Create source
	data := []byte("move test data")
	w, _ := backend.NewWriter(ctx, "move-src.txt")
	_, _ = w.Write(data)
	_ = w.Close()

	// Move
	if err := backend.Move(ctx, "move-src.txt", "move-dst.txt"); err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Verify destination
	r, err := backend.NewReader(ctx, "move-dst.txt")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	readData, _ := io.ReadAll(r)
	_ = r.Close()

	if string(readData) != string(data) {
		t.Errorf("Moved data = %q, want %q", readData, data)
	}

	// Verify source is gone
	exists, _ := backend.Exists(ctx, "move-src.txt")
	if exists {
		t.Error("Source should not exist after move")
	}

	// Cleanup
	_ = backend.Delete(ctx, "move-dst.txt")
}

func TestIntegrationRangeRead(t *testing.T) {
	backend := getTestBackend(t)
	defer func() { _ = backend.Close() }()

	ctx := context.Background()

	// Create file
	data := []byte("hello world range test")
	w, _ := backend.NewWriter(ctx, "range-test.txt")
	_, _ = w.Write(data)
	_ = w.Close()

	// Read with offset
	r, err := backend.NewReader(ctx, "range-test.txt", omnistorage.WithOffset(6), omnistorage.WithLimit(5))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	readData, _ := io.ReadAll(r)
	_ = r.Close()

	if string(readData) != "world" {
		t.Errorf("Range read = %q, want %q", readData, "world")
	}

	// Cleanup
	_ = backend.Delete(ctx, "range-test.txt")
}

func TestIntegrationRegistry(t *testing.T) {
	// S3 backend should be registered
	if !omnistorage.IsRegistered("s3") {
		t.Error("s3 backend should be registered")
	}
}

func TestCheckClosed(t *testing.T) {
	backend := &Backend{
		config: Config{Bucket: "test"},
	}

	// Initially not closed
	err := backend.checkClosed()
	if err != nil {
		t.Errorf("checkClosed() on open backend = %v, want nil", err)
	}

	// Close the backend
	if err := backend.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	// Should be closed now
	err = backend.checkClosed()
	if err != omnistorage.ErrBackendClosed {
		t.Errorf("checkClosed() on closed backend = %v, want ErrBackendClosed", err)
	}

	// Close again should be safe
	if err := backend.Close(); err != nil {
		t.Errorf("Close() on already closed = %v, want nil", err)
	}
}

func TestTranslateError(t *testing.T) {
	backend := &Backend{
		config: Config{Bucket: "test-bucket"},
	}

	tests := []struct {
		name     string
		err      error
		wantErr  error
		wantWrap bool
	}{
		{
			name:    "nil error",
			err:     nil,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backend.translateError(tt.err, "test-path")
			if got != tt.wantErr {
				t.Errorf("translateError() = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestConfigFromMapEdgeCases(t *testing.T) {
	// Test with empty map
	cfg := ConfigFromMap(map[string]string{})
	if cfg.PartSize != 5*1024*1024 {
		t.Errorf("Empty map should use default PartSize, got %d", cfg.PartSize)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("Empty map should use default Concurrency, got %d", cfg.Concurrency)
	}

	// Test with invalid part_size (should keep default)
	cfg = ConfigFromMap(map[string]string{
		"bucket":    "test",
		"part_size": "invalid",
	})
	if cfg.PartSize != 5*1024*1024 {
		t.Errorf("Invalid part_size should use default, got %d", cfg.PartSize)
	}

	// Test with zero part_size (should keep default)
	cfg = ConfigFromMap(map[string]string{
		"bucket":    "test",
		"part_size": "0",
	})
	if cfg.PartSize != 5*1024*1024 {
		t.Errorf("Zero part_size should use default, got %d", cfg.PartSize)
	}

	// Test with negative part_size (should keep default)
	cfg = ConfigFromMap(map[string]string{
		"bucket":    "test",
		"part_size": "-100",
	})
	if cfg.PartSize != 5*1024*1024 {
		t.Errorf("Negative part_size should use default, got %d", cfg.PartSize)
	}

	// Test with invalid concurrency (should keep default)
	cfg = ConfigFromMap(map[string]string{
		"bucket":      "test",
		"concurrency": "invalid",
	})
	if cfg.Concurrency != 5 {
		t.Errorf("Invalid concurrency should use default, got %d", cfg.Concurrency)
	}

	// Test with zero concurrency (should keep default)
	cfg = ConfigFromMap(map[string]string{
		"bucket":      "test",
		"concurrency": "0",
	})
	if cfg.Concurrency != 5 {
		t.Errorf("Zero concurrency should use default, got %d", cfg.Concurrency)
	}

	// Test "1" as boolean value for use_path_style
	cfg = ConfigFromMap(map[string]string{
		"bucket":         "test",
		"use_path_style": "1",
	})
	if !cfg.UsePathStyle {
		t.Error("use_path_style '1' should set UsePathStyle to true")
	}

	// Test "1" as boolean value for disable_ssl
	cfg = ConfigFromMap(map[string]string{
		"bucket":      "test",
		"disable_ssl": "1",
	})
	if !cfg.DisableSSL {
		t.Error("disable_ssl '1' should set DisableSSL to true")
	}

	// Test session_token
	cfg = ConfigFromMap(map[string]string{
		"bucket":        "test",
		"session_token": "my-token",
	})
	if cfg.SessionToken != "my-token" {
		t.Errorf("SessionToken = %q, want %q", cfg.SessionToken, "my-token")
	}
}

func TestNewValidation(t *testing.T) {
	// Test that New fails with invalid config
	_, err := New(Config{})
	if err == nil {
		t.Error("New() with empty bucket should fail")
	}
	if err != ErrBucketRequired {
		t.Errorf("New() = %v, want ErrBucketRequired", err)
	}
}

func TestFullKeyEdgeCases(t *testing.T) {
	tests := []struct {
		prefix   string
		path     string
		expected string
	}{
		{"", "", ""},
		{"", "/", "/"},
		{"prefix/", "/file.txt", "prefix/file.txt"},
		{"prefix//", "file.txt", "prefix/file.txt"},
	}

	for _, tt := range tests {
		backend := &Backend{config: Config{Prefix: tt.prefix}}
		result := backend.fullKey(tt.path)
		if result != tt.expected {
			t.Errorf("fullKey(%q) with prefix %q = %q, want %q", tt.path, tt.prefix, result, tt.expected)
		}
	}
}

func TestFeaturesDetails(t *testing.T) {
	backend := &Backend{config: Config{Bucket: "test"}}
	features := backend.Features()

	// Test all boolean features
	if !features.Mkdir {
		t.Error("Features.Mkdir = false, want true")
	}
	if !features.Rmdir {
		t.Error("Features.Rmdir = false, want true")
	}
	if !features.CanStream {
		t.Error("Features.CanStream = false, want true")
	}
	if !features.ServerSideEncryption {
		t.Error("Features.ServerSideEncryption = false, want true")
	}
	if !features.Versioning {
		t.Error("Features.Versioning = false, want true")
	}

	// Test that SHA256 is not supported
	if features.SupportsHash(omnistorage.HashSHA256) {
		t.Error("Features should not support SHA256 hash")
	}
}

func TestErrBucketRequired(t *testing.T) {
	// Verify the error message
	if ErrBucketRequired.Error() != "s3: bucket is required" {
		t.Errorf("ErrBucketRequired.Error() = %q, want %q",
			ErrBucketRequired.Error(), "s3: bucket is required")
	}
}
