# Omni-AWS

AWS providers for the PlexusOne ecosystem, covering LLM access, email, storage,
secrets, memory, and developer-experience telemetry.

## Packages

| Package | Description | Import Path |
|---------|-------------|-------------|
| **omnillm** | AWS Bedrock provider for OmniLLM | `github.com/plexusone/omni-aws/omnillm` |
| **omnimail** | Amazon SES v2 sender for OmniMail | `github.com/plexusone/omni-aws/omnimail` |
| **omnimemory** | DynamoDB provider for OmniMemory | `github.com/plexusone/omni-aws/omnimemory/dynamodb` |
| **omnistorage** | S3 backend for OmniStorage | `github.com/plexusone/omni-aws/omnistorage/backend/s3` |
| **omnivault** | Secrets Manager & Parameter Store for OmniVault | `github.com/plexusone/omni-aws/omnivault` |
| **omnidevx** | Kiro CLI telemetry collector for OmniDevX | `github.com/plexusone/omni-aws/omnidevx` |

## Developer Experience

The OmniDevX package imports Kiro CLI local history into the shared
`omnidevx-core` event model. It is intended for historical reporting over AI
assistant usage, token consumption, sessions, and tool activity.

```go
import (
    kiro "github.com/plexusone/omni-aws/omnidevx"
    core "github.com/plexusone/omnidevx-core"
)

collector, err := kiro.New(kiro.Config{})
result, err := collector.Collect(ctx, core.CollectRequest{
    Subject: core.SubjectRef{PersonID: "person:jane"},
})
```

See [OmniDevX: Kiro CLI Collector](omnidevx/index.md) for local paths,
format-stability notes, and event mapping.

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

### OmniMail (SES)

```go
import ses "github.com/plexusone/omni-aws/omnimail"

sender, err := ses.New(ses.Config{
    Region: "us-east-1",
})
```

See [OmniMail: Amazon SES](omnimail/index.md) for configuration, error
mapping, IAM and SES domain setup.

### OmniMemory (DynamoDB)

```go
import (
    "github.com/plexusone/omnimemory"
    "github.com/plexusone/omnimemory/core"
    _ "github.com/plexusone/omni-aws/omnimemory/dynamodb"
)

client, _ := omnimemory.NewClient(core.ClientConfig{
    Providers: []core.ProviderConfig{
        {Name: core.ProviderNameAWSDynamoDB, Options: map[string]any{
            "table_name": "omnimemory",
            "region":     "us-east-1",
        }},
    },
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
