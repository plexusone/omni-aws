# omni-aws

AWS providers for the PlexusOne ecosystem.

## Packages

| Package | Description | Import Path |
|---------|-------------|-------------|
| **omnillm** | AWS Bedrock provider for OmniLLM | `github.com/plexusone/omni-aws/omnillm` |
| **omnistorage** | S3 backend for OmniStorage | `github.com/plexusone/omni-aws/omnistorage/backend/s3` |
| **omnivault** | Secrets Manager & Parameter Store for OmniVault | `github.com/plexusone/omni-aws/omnivault` |

## Installation

```bash
go get github.com/plexusone/omni-aws
```

## Quick Start

### OmniLLM (Bedrock)

```go
import "github.com/plexusone/omni-aws/omnillm"

provider, err := bedrock.New(bedrock.Config{
    Region: "us-east-1",
})
```

### OmniStorage (S3)

```go
import "github.com/plexusone/omni-aws/omnistorage/backend/s3"

backend, err := s3.New(s3.Config{
    Bucket: "my-bucket",
    Region: "us-east-1",
})
```

### OmniVault (Secrets Manager)

```go
import aws "github.com/plexusone/omni-aws/omnivault"

provider, err := aws.NewSecretsManager(aws.Config{
    Region: "us-east-1",
})
```

## Links

- [GitHub Repository](https://github.com/plexusone/omni-aws)
- [Go Package Documentation](https://pkg.go.dev/github.com/plexusone/omni-aws)
- [Release Notes](releases/index.md)
- [Changelog](https://github.com/plexusone/omni-aws/blob/main/CHANGELOG.md)
