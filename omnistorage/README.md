# omnistorage

S3 backend for [omnistorage-core](https://github.com/plexusone/omnistorage-core).

## Installation

```bash
go get github.com/plexusone/omni-aws/omnistorage
```

## S3 Backend

The S3 backend implements `omnistorage.ExtendedBackend` for S3-compatible object storage.

### Supported Services

- AWS S3
- Cloudflare R2
- MinIO
- Wasabi
- DigitalOcean Spaces
- Any S3-compatible object storage

### Basic Usage

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

### S3-Compatible Services

```go
// MinIO
backend, err := s3.New(s3.Config{
    Bucket:       "my-bucket",
    Endpoint:     "http://localhost:9000",
    UsePathStyle: true,
    AccessKeyID:     "minioadmin",
    SecretAccessKey: "minioadmin",
})

// Cloudflare R2
backend, err := s3.New(s3.Config{
    Bucket:   "my-bucket",
    Endpoint: "https://<account_id>.r2.cloudflarestorage.com",
    AccessKeyID:     "...",
    SecretAccessKey: "...",
})

// Wasabi
backend, err := s3.New(s3.Config{
    Bucket:   "my-bucket",
    Endpoint: "https://s3.wasabisys.com",
    Region:   "us-east-1",
})
```

### Environment Variables

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

### Backend Registry

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

### Features

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
