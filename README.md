# omni-aws

AWS provider packages for [PlexusOne](https://github.com/plexusone) libraries.

## Modules

This repository contains multiple Go modules for AWS integrations:

| Module | Description | Install |
|--------|-------------|---------|
| [`omnillm`](omnillm/) | AWS Bedrock provider for [omnillm-core](https://github.com/plexusone/omnillm-core) | `go get github.com/plexusone/omni-aws/omnillm` |
| [`omnistorage`](omnistorage/) | S3 backend for [omnistorage-core](https://github.com/plexusone/omnistorage-core) | `go get github.com/plexusone/omni-aws/omnistorage` |

## Quick Start

### OmniLLM - AWS Bedrock

```go
import (
    "github.com/plexusone/omni-aws/omnillm"
    "github.com/plexusone/omnillm-core"
)

// Create Bedrock provider
provider := omnillm.NewProvider("us-east-1")

// Use with OmniLLM
client := omnillm.NewClient(provider)
resp, err := client.CreateChatCompletion(ctx, omnillm.ChatCompletionRequest{
    Model: "anthropic.claude-3-5-sonnet-20241022-v2:0",
    Messages: []omnillm.Message{
        {Role: "user", Content: "Hello!"},
    },
})
```

See [omnillm/README.md](omnillm/README.md) for full documentation.

### OmniStorage - S3 Backend

```go
import (
    "github.com/plexusone/omni-aws/omnistorage/backend/s3"
)

// Create S3 backend
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

See [omnistorage/README.md](omnistorage/) for full documentation including S3-compatible services (R2, MinIO, Wasabi).

## License

MIT
