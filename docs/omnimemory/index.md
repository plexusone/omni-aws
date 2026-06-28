# OmniMemory DynamoDB Provider

DynamoDB provider for [omnimemory](https://github.com/plexusone/omnimemory).

## Installation

```bash
go get github.com/plexusone/omni-aws
```

## Overview

The DynamoDB provider implements `omnimemory.Provider` for serverless memory storage with automatic scaling and TTL support.

### Features

- Fully managed, serverless storage
- Automatic scaling (pay-per-request billing)
- Built-in TTL for memory expiration
- Multi-tenant isolation via partition keys
- Auto-create table option for development
- Custom endpoint support for DynamoDB Local

## Basic Usage

```go
import (
    "github.com/plexusone/omnimemory"
    "github.com/plexusone/omnimemory/core"
    _ "github.com/plexusone/omni-aws/omnimemory/dynamodb"
)

client, err := omnimemory.NewClient(core.ClientConfig{
    Providers: []core.ProviderConfig{
        {
            Name: core.ProviderNameAWSDynamoDB,
            Options: map[string]any{
                "table_name": "omnimemory",
                "region":     "us-east-1",
            },
        },
    },
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Add a memory
memory, err := client.Add(ctx, &core.AddRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    Type:    core.MemoryTypeObservation,
    Content: "User prefers dark mode interfaces",
})

// Search memories
results, err := client.Search(ctx, &core.SearchRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    Query: "interface preferences",
    Limit: 10,
})
```

## Configuration Options

```go
{
    Name: core.ProviderNameAWSDynamoDB,
    Options: map[string]any{
        "table_name":   "omnimemory",    // Required: DynamoDB table name
        "region":       "us-east-1",      // Optional: AWS region
        "endpoint":     "http://...",     // Optional: Custom endpoint (DynamoDB Local)
        "create_table": true,             // Optional: Auto-create table (default: false)
    },
}
```

| Option | Type | Description |
|--------|------|-------------|
| `table_name` | string | DynamoDB table name (required) |
| `region` | string | AWS region (optional, uses default config) |
| `endpoint` | string | Custom endpoint URL for DynamoDB Local |
| `create_table` | bool | Auto-create table if not exists (default: false) |

## Table Schema

The provider uses a single-table design optimized for tenant isolation and efficient queries:

| Attribute | Type | Description |
|-----------|------|-------------|
| `pk` (Partition Key) | String | `tenant_id` - isolates data by tenant |
| `sk` (Sort Key) | String | `subject_id#memory_id` - enables subject queries |
| `expires_at` | Number | TTL attribute (Unix timestamp) |

### Additional Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | Memory UUID |
| `tenant_id` | Tenant identifier |
| `subject_id` | Subject (user) identifier |
| `agent_id` | Agent that created the memory |
| `session_id` | Session identifier |
| `scope` | Memory scope (user, agent, tenant, etc.) |
| `type` | Memory type (observation, fact, preference, etc.) |
| `content` | Memory content text |
| `embedding` | JSON-encoded embedding vector |
| `metadata` | JSON-encoded metadata |
| `created_at` | Creation timestamp (Unix) |
| `updated_at` | Last update timestamp (Unix) |

## Local Development

Use DynamoDB Local for development without AWS costs:

```bash
# Start DynamoDB Local with Docker
docker run -p 8000:8000 amazon/dynamodb-local
```

```go
client, err := omnimemory.NewClient(core.ClientConfig{
    Providers: []core.ProviderConfig{
        {
            Name: core.ProviderNameAWSDynamoDB,
            Options: map[string]any{
                "table_name":   "omnimemory",
                "endpoint":     "http://localhost:8000",
                "create_table": true,
            },
        },
    },
})
```

## Environment Variables

The provider uses standard AWS SDK environment variables:

| Variable | Description |
|----------|-------------|
| `AWS_REGION` | AWS region |
| `AWS_ACCESS_KEY_ID` | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key |
| `AWS_SESSION_TOKEN` | Session token (optional) |

## Semantic Search

Since DynamoDB doesn't support native vector search, the provider performs in-memory cosine similarity:

1. Query retrieves all memories for the tenant/subject
2. Embeddings are compared using cosine similarity
3. Results are sorted by score and filtered by threshold

This approach works well for moderate data sizes. For large-scale vector search, consider:

- PostgreSQL with pgvector
- AWS OpenSearch with k-NN (planned)

## Memory Expiration (TTL)

Memories can expire automatically using DynamoDB's TTL feature:

```go
memory, err := client.Add(ctx, &core.AddRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    Type:    core.MemoryTypeObservation,
    Content: "Temporary observation",
    TTL:     24 * time.Hour, // Expires in 24 hours
})
```

The `expires_at` attribute is set automatically and DynamoDB handles deletion.

## IAM Permissions

### Minimal Permissions

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "dynamodb:GetItem",
                "dynamodb:PutItem",
                "dynamodb:UpdateItem",
                "dynamodb:DeleteItem",
                "dynamodb:Query"
            ],
            "Resource": "arn:aws:dynamodb:*:*:table/omnimemory"
        }
    ]
}
```

### With Auto-Create Table

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "dynamodb:GetItem",
                "dynamodb:PutItem",
                "dynamodb:UpdateItem",
                "dynamodb:DeleteItem",
                "dynamodb:Query",
                "dynamodb:CreateTable",
                "dynamodb:DescribeTable",
                "dynamodb:UpdateTimeToLive"
            ],
            "Resource": "arn:aws:dynamodb:*:*:table/omnimemory"
        }
    ]
}
```

## Operations

### Add Memory

```go
memory, err := client.Add(ctx, &core.AddRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
        AgentID:   "assistant",
    },
    Type:     core.MemoryTypeObservation,
    Content:  "User prefers dark mode",
    Metadata: map[string]any{"source": "chat"},
    TTL:      7 * 24 * time.Hour,
})
```

### Get Memory

```go
memory, err := client.Get(ctx, &core.GetRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    ID: "memory-uuid",
})
```

### Update Memory

```go
memory, err := client.Update(ctx, &core.UpdateRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    ID:      "memory-uuid",
    Content: "Updated content",
})
```

### Delete Memory

```go
err := client.Delete(ctx, &core.DeleteRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    ID: "memory-uuid",
})
```

### List Memories

```go
resp, err := client.List(ctx, &core.ListRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    Types:  []core.MemoryType{core.MemoryTypeObservation},
    Limit:  50,
})

for _, mem := range resp.Memories {
    fmt.Printf("- %s: %s\n", mem.ID, mem.Content)
}
```

### Search Memories

```go
resp, err := client.Search(ctx, &core.SearchRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    Query:     "user preferences",
    Limit:     10,
    Threshold: 0.7,
})

for _, r := range resp.Results {
    fmt.Printf("Score: %.2f - %s\n", r.Score, r.Memory.Content)
}
```

### Recall Memories

```go
resp, err := client.Recall(ctx, &core.RecallRequest{
    Context: core.Context{
        TenantID:  "tenant-123",
        SubjectID: "user-456",
    },
    Query:      "What does the user prefer?",
    MaxResults: 5,
})

for _, mem := range resp.Memories {
    fmt.Printf("- %s\n", mem.Content)
}
```

## Multi-Tenancy

The DynamoDB provider enforces strict tenant isolation via the partition key:

```go
// Tenant A
client.Add(ctx, &core.AddRequest{
    Context: core.Context{
        TenantID:  "tenant-a",
        SubjectID: "user-123",
    },
    Content: "Memory for tenant A",
})

// Tenant B (completely isolated)
client.Add(ctx, &core.AddRequest{
    Context: core.Context{
        TenantID:  "tenant-b",
        SubjectID: "user-123", // Same user ID, different tenant
    },
    Content: "Memory for tenant B",
})
```

Queries always include the partition key, ensuring tenants cannot access each other's data.

## Related

- [omnimemory Documentation](https://plexusone.github.io/omnimemory/)
- [DynamoDB Developer Guide](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/)
- [DynamoDB Local](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
