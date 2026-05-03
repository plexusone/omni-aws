# OmniStorage S3 Backend

S3 backend for [omnistorage-core](https://github.com/plexusone/omnistorage-core).

## Installation

```bash
go get github.com/plexusone/omni-aws
```

## Overview

The S3 backend implements `omnistorage.ExtendedBackend` for S3-compatible object storage.

### Supported Services

- AWS S3
- Cloudflare R2
- MinIO
- Wasabi
- DigitalOcean Spaces
- Any S3-compatible object storage

## Basic Usage

```go
import (
    "github.com/plexusone/omni-aws/omnistorage/backend/s3"
)

// AWS S3
backend, err := s3.New(s3.Config{
    Bucket: "my-bucket",
    Region: "us-east-1",
})

// Write
w, _ := backend.NewWriter(ctx, "path/to/file.txt")
w.Write([]byte("hello"))
w.Close()

// Read
r, _ := backend.NewReader(ctx, "path/to/file.txt")
data, _ := io.ReadAll(r)
r.Close()
```

## S3-Compatible Services

### MinIO

```go
backend, err := s3.New(s3.Config{
    Bucket:          "my-bucket",
    Endpoint:        "http://localhost:9000",
    UsePathStyle:    true,
    AccessKeyID:     "minioadmin",
    SecretAccessKey: "minioadmin",
})
```

### Cloudflare R2

```go
backend, err := s3.New(s3.Config{
    Bucket:          "my-bucket",
    Endpoint:        "https://<account_id>.r2.cloudflarestorage.com",
    AccessKeyID:     "...",
    SecretAccessKey: "...",
})
```

### Wasabi

```go
backend, err := s3.New(s3.Config{
    Bucket:   "my-bucket",
    Endpoint: "https://s3.wasabisys.com",
    Region:   "us-east-1",
})
```

### DigitalOcean Spaces

```go
backend, err := s3.New(s3.Config{
    Bucket:          "my-bucket",
    Endpoint:        "https://nyc3.digitaloceanspaces.com",
    Region:          "nyc3",
    AccessKeyID:     "...",
    SecretAccessKey: "...",
})
```

## Environment Variables

Use `s3.ConfigFromEnv()` to configure from environment variables:

| Variable | Description |
|----------|-------------|
| `OMNISTORAGE_S3_BUCKET` | Bucket name |
| `OMNISTORAGE_S3_REGION` | AWS region |
| `OMNISTORAGE_S3_ENDPOINT` | Custom endpoint URL |
| `OMNISTORAGE_S3_PREFIX` | Key prefix |
| `OMNISTORAGE_S3_USE_PATH_STYLE` | Set to `true` for path-style addressing |
| `OMNISTORAGE_S3_DISABLE_SSL` | Set to `true` to disable SSL |
| `AWS_ACCESS_KEY_ID` | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key |
| `AWS_SESSION_TOKEN` | Session token (optional) |
| `AWS_REGION` | Fallback for region |

```go
cfg := s3.ConfigFromEnv()
backend, err := s3.New(cfg)
```

## Backend Registry

The S3 backend auto-registers with omnistorage's backend registry:

```go
import (
    omnistorage "github.com/plexusone/omnistorage-core/object"
    _ "github.com/plexusone/omni-aws/omnistorage/backend/s3" // Register s3 backend
)

backend, err := omnistorage.Open("s3", map[string]string{
    "bucket": "my-bucket",
    "region": "us-east-1",
})
```

## Features

The S3 backend supports:

- Streaming read/write with multipart uploads
- Server-side copy and move
- Range read requests for partial content
- MD5 hash via ETag
- Configurable concurrency and part size
- Directory markers via `Mkdir`/`Rmdir`

```go
// Check capabilities
features := backend.Features()
// features.CanCopy = true
// features.CanMove = true
// features.CanMkdir = true
// features.CanStat = true
// features.Hashes = ["md5"]
```

## Operations

### Write Files

```go
// Simple write
err := backend.Write(ctx, "path/to/file.txt", reader, size)

// Streaming write with multipart upload
w, err := backend.NewWriter(ctx, "path/to/large-file.bin")
if err != nil {
    return err
}
defer w.Close()

// Write in chunks
for chunk := range chunks {
    w.Write(chunk)
}
```

### Read Files

```go
// Simple read
reader, err := backend.NewReader(ctx, "path/to/file.txt")
if err != nil {
    return err
}
defer reader.Close()

data, err := io.ReadAll(reader)
```

### List Files

```go
entries, err := backend.List(ctx, "path/prefix/")
for _, entry := range entries {
    fmt.Printf("%s (size: %d, dir: %t)\n", entry.Name, entry.Size, entry.IsDir)
}
```

### Copy and Move

```go
// Server-side copy (no data transfer through client)
err := backend.Copy(ctx, "source/path.txt", "dest/path.txt")

// Move (copy + delete)
err := backend.Move(ctx, "old/path.txt", "new/path.txt")
```

### Delete

```go
// Delete single file
err := backend.Delete(ctx, "path/to/file.txt")

// Delete with prefix (all matching objects)
err := backend.DeletePrefix(ctx, "path/to/delete/")
```

## Configuration Options

```go
type Config struct {
    // Required
    Bucket string

    // AWS Configuration
    Region          string
    AccessKeyID     string
    SecretAccessKey string
    SessionToken    string

    // S3-Compatible Services
    Endpoint     string // Custom endpoint URL
    UsePathStyle bool   // Use path-style addressing (required for MinIO)
    DisableSSL   bool   // Disable SSL/TLS

    // Optional
    Prefix string // Key prefix for all operations

    // Performance
    PartSize    int64 // Multipart upload part size (default: 5MB)
    Concurrency int   // Upload/download concurrency (default: 5)
}
```

## IAM Permissions

Required IAM policy for full access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:GetBucketLocation"
      ],
      "Resource": [
        "arn:aws:s3:::my-bucket",
        "arn:aws:s3:::my-bucket/*"
      ]
    }
  ]
}
```

For read-only access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::my-bucket",
        "arn:aws:s3:::my-bucket/*"
      ]
    }
  ]
}
```
