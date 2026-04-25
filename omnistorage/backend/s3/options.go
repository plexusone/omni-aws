package s3

import (
	"os"
	"strconv"
)

// Config holds configuration for the S3 backend.
type Config struct {
	// Bucket is the S3 bucket name (required).
	Bucket string

	// Region is the AWS region (e.g., "us-east-1").
	// If empty, uses AWS_REGION or AWS_DEFAULT_REGION environment variable.
	Region string

	// Endpoint is a custom endpoint URL for S3-compatible services.
	// Examples:
	//   - MinIO: "http://localhost:9000"
	//   - Cloudflare R2: "https://<account_id>.r2.cloudflarestorage.com"
	//   - Wasabi: "https://s3.wasabisys.com"
	// Leave empty for AWS S3.
	Endpoint string

	// Prefix is an optional prefix for all keys.
	// Useful for organizing data within a bucket.
	Prefix string

	// AccessKeyID is the AWS access key ID.
	// If empty, uses AWS_ACCESS_KEY_ID environment variable or IAM role.
	AccessKeyID string

	// SecretAccessKey is the AWS secret access key.
	// If empty, uses AWS_SECRET_ACCESS_KEY environment variable or IAM role.
	SecretAccessKey string

	// SessionToken is an optional session token for temporary credentials.
	SessionToken string

	// UsePathStyle forces path-style addressing instead of virtual-hosted-style.
	// Required for some S3-compatible services like MinIO.
	// Set to true for: MinIO, some older S3-compatible services.
	// Set to false for: AWS S3, Cloudflare R2, Wasabi.
	UsePathStyle bool

	// DisableSSL disables HTTPS for the endpoint.
	// Only use for local development (e.g., local MinIO).
	DisableSSL bool

	// PartSize is the size in bytes for multipart upload parts.
	// Default: 5MB (minimum for S3).
	// Increase for better performance with large files.
	PartSize int64

	// Concurrency is the number of concurrent upload/download goroutines.
	// Default: 5.
	Concurrency int
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		PartSize:    5 * 1024 * 1024, // 5MB
		Concurrency: 5,
	}
}

// ConfigFromEnv creates a Config from environment variables.
// Environment variables:
//   - OMNISTORAGE_S3_BUCKET or AWS_S3_BUCKET: bucket name
//   - OMNISTORAGE_S3_REGION or AWS_REGION or AWS_DEFAULT_REGION: region
//   - OMNISTORAGE_S3_ENDPOINT: custom endpoint
//   - OMNISTORAGE_S3_PREFIX: key prefix
//   - AWS_ACCESS_KEY_ID: access key
//   - AWS_SECRET_ACCESS_KEY: secret key
//   - AWS_SESSION_TOKEN: session token
//   - OMNISTORAGE_S3_USE_PATH_STYLE: "true" for path-style addressing
//   - OMNISTORAGE_S3_DISABLE_SSL: "true" to disable SSL
func ConfigFromEnv() Config {
	config := DefaultConfig()

	// Bucket
	if v := os.Getenv("OMNISTORAGE_S3_BUCKET"); v != "" {
		config.Bucket = v
	} else if v := os.Getenv("AWS_S3_BUCKET"); v != "" {
		config.Bucket = v
	}

	// Region
	if v := os.Getenv("OMNISTORAGE_S3_REGION"); v != "" {
		config.Region = v
	} else if v := os.Getenv("AWS_REGION"); v != "" {
		config.Region = v
	} else if v := os.Getenv("AWS_DEFAULT_REGION"); v != "" {
		config.Region = v
	}

	// Endpoint
	if v := os.Getenv("OMNISTORAGE_S3_ENDPOINT"); v != "" {
		config.Endpoint = v
	}

	// Prefix
	if v := os.Getenv("OMNISTORAGE_S3_PREFIX"); v != "" {
		config.Prefix = v
	}

	// Credentials from environment (AWS SDK will also pick these up)
	config.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	config.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	config.SessionToken = os.Getenv("AWS_SESSION_TOKEN")

	// Path style
	if v := os.Getenv("OMNISTORAGE_S3_USE_PATH_STYLE"); v == "true" || v == "1" {
		config.UsePathStyle = true
	}

	// Disable SSL
	if v := os.Getenv("OMNISTORAGE_S3_DISABLE_SSL"); v == "true" || v == "1" {
		config.DisableSSL = true
	}

	return config
}

// ConfigFromMap creates a Config from a string map.
// Supported keys:
//   - bucket: bucket name (required)
//   - region: AWS region
//   - endpoint: custom endpoint URL
//   - prefix: key prefix
//   - access_key_id: AWS access key
//   - secret_access_key: AWS secret key
//   - session_token: session token
//   - use_path_style: "true" for path-style addressing
//   - disable_ssl: "true" to disable SSL
//   - part_size: multipart upload part size in bytes
//   - concurrency: number of concurrent operations
func ConfigFromMap(m map[string]string) Config {
	config := DefaultConfig()

	if v, ok := m["bucket"]; ok {
		config.Bucket = v
	}
	if v, ok := m["region"]; ok {
		config.Region = v
	}
	if v, ok := m["endpoint"]; ok {
		config.Endpoint = v
	}
	if v, ok := m["prefix"]; ok {
		config.Prefix = v
	}
	if v, ok := m["access_key_id"]; ok {
		config.AccessKeyID = v
	}
	if v, ok := m["secret_access_key"]; ok {
		config.SecretAccessKey = v
	}
	if v, ok := m["session_token"]; ok {
		config.SessionToken = v
	}
	if v, ok := m["use_path_style"]; ok && (v == "true" || v == "1") {
		config.UsePathStyle = true
	}
	if v, ok := m["disable_ssl"]; ok && (v == "true" || v == "1") {
		config.DisableSSL = true
	}
	if v, ok := m["part_size"]; ok {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil && size > 0 {
			config.PartSize = size
		}
	}
	if v, ok := m["concurrency"]; ok {
		if c, err := strconv.Atoi(v); err == nil && c > 0 {
			config.Concurrency = c
		}
	}

	return config
}

// Validate checks if the configuration is valid.
func (c Config) Validate() error {
	if c.Bucket == "" {
		return ErrBucketRequired
	}
	return nil
}
