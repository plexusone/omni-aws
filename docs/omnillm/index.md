# OmniLLM Provider for AWS Bedrock

[OmniLLM](https://github.com/plexusone/omnillm-core) Provider for AWS Bedrock is an external provider module that enables using AWS Bedrock foundation models with OmniLLM.

## Installation

```bash
go get github.com/plexusone/omni-aws
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/plexusone/omnillm-core"
    "github.com/plexusone/omni-aws/omnillm"
    "github.com/plexusone/omnillm-core/provider"
)

func main() {
    // Create the Bedrock provider
    bedrockProvider, err := bedrock.NewProvider("us-east-1")
    if err != nil {
        log.Fatal(err)
    }

    // Use it with omnillm via CustomProvider
    client, err := omnillm.NewClient(omnillm.ClientConfig{
        CustomProvider: bedrockProvider,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Make requests as usual
    resp, err := client.CreateChatCompletion(context.Background(), &provider.ChatCompletionRequest{
        Model: "anthropic.claude-3-sonnet-20240229-v1:0",
        Messages: []provider.Message{
            {Role: provider.RoleUser, Content: "Hello!"},
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Println(resp.Choices[0].Message.Content)
}
```

## AWS Authentication

Credentials are resolved in priority order:

| Priority | Source | Description | Use Case |
|----------|--------|-------------|----------|
| 1 | **Bedrock API Key** | `AWS_BEARER_TOKEN_BEDROCK` | Simplest setup, dev/prototyping |
| 2 | Environment variables | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` | CI/CD pipelines, containers |
| 3 | Shared credentials file | `~/.aws/credentials` with optional `AWS_PROFILE` | Local development |
| 4 | Shared config file | `~/.aws/config` (profiles, SSO, assume role) | Local development with SSO |
| 5 | EC2 instance metadata | IMDS (Instance Metadata Service) | EC2 instances |
| 6 | ECS container credentials | Task IAM role via `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` | ECS/Fargate tasks |
| 7 | EKS Pod Identity / IRSA | Web identity token via `AWS_WEB_IDENTITY_TOKEN_FILE` | Kubernetes workloads |

### Bedrock API Key (Recommended for Development)

The simplest authentication method. Generate a key in the [AWS Console](https://console.aws.amazon.com/bedrock/) under **API keys**:

```bash
export AWS_BEARER_TOKEN_BEDROCK="your-bedrock-api-key"
```

!!! note
    For production, use IAM credentials or instance roles for better security and auditability.

### IAM Credentials

Set environment variables:

```bash
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
```

Or use AWS profiles:

```bash
export AWS_PROFILE=my-profile
```

### Production (EKS with IRSA)

For Kubernetes workloads, use [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html):

1. Create an IAM role with the required policy (see below)
2. Associate the role with a Kubernetes service account
3. The SDK automatically uses the web identity token

### Required IAM Permissions

The IAM user or role must have permissions to invoke Bedrock models:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "bedrock:InvokeModel",
      "bedrock:InvokeModelWithResponseStream"
    ],
    "Resource": "arn:aws:bedrock:*::foundation-model/*"
  }]
}
```

### Bedrock Model Access

Serverless foundation models are automatically enabled on first invocation in AWS commercial regions. No manual setup required.

!!! note
    Some models (e.g., certain Anthropic models) may require accepting an EULA in the AWS Console under **Bedrock → Model access** before first use.

## Streaming

Stream responses token-by-token for responsive UIs:

```go
stream, err := provider.CreateChatCompletionStream(ctx, &provider.ChatCompletionRequest{
    Model: "anthropic.claude-3-haiku-20240307-v1:0",
    Messages: []provider.Message{
        {Role: provider.RoleUser, Content: "Tell me a story"},
    },
})
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    fmt.Print(chunk.Choices[0].Delta.Content)
}
```

## Tool Calling

Define tools and let the model decide when to use them:

```go
resp, err := provider.CreateChatCompletion(ctx, &provider.ChatCompletionRequest{
    Model: "anthropic.claude-3-haiku-20240307-v1:0",
    Messages: []provider.Message{
        {Role: provider.RoleUser, Content: "What's the weather in Seattle?"},
    },
    Tools: []provider.Tool{{
        Type: "function",
        Function: provider.ToolSpec{
            Name:        "get_weather",
            Description: "Get current weather for a location",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "location": map[string]any{"type": "string"},
                },
                "required": []string{"location"},
            },
        },
    }},
    ToolChoice: "auto",
})
```

Tool calling also works with streaming for real-time tool use detection.

## Testing

### Unit Tests

```bash
go test -v ./omnillm/...
```

### Integration Tests

Integration tests make real API calls to AWS Bedrock. Configure credentials and run with the `integration` build tag:

```bash
go test -tags=integration -v ./omnillm/...
```

#### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AWS_BEARER_TOKEN_BEDROCK` | Option 1 | - | Bedrock API key (simplest) |
| `AWS_ACCESS_KEY_ID` | Option 2 | - | AWS access key |
| `AWS_SECRET_ACCESS_KEY` | Option 2 | - | AWS secret key |
| `AWS_SESSION_TOKEN` | No | - | Session token (for temporary credentials) |
| `BEDROCK_TEST_REGION` | No | `us-east-1` | AWS region for tests |
| `BEDROCK_TEST_MODEL` | No | `anthropic.claude-3-haiku-20240307-v1:0` | Model ID for tests |

Use **either** `AWS_BEARER_TOKEN_BEDROCK` (Option 1) **or** IAM credentials (Option 2).

## Feature Support

| Feature | Supported |
|---------|-----------|
| Chat Completion | Yes |
| Streaming | Yes |
| Tool Calling | Yes |
| System Messages | Yes |
| JSON Mode | No |

Works with all Bedrock models via the Converse API.

## Why a Separate Module?

The AWS SDK v2 pulls in 17+ transitive dependencies (credentials, STS, SSO, EC2 IMDS, etc.). By keeping Bedrock in a separate module, users who don't need AWS integration avoid downloading and compiling these dependencies.

## Creating Your Own External Provider

To create a custom provider for omnillm:

1. Implement the `provider.Provider` interface from `github.com/plexusone/omnillm-core/provider`
2. Use `omnillm.ClientConfig.CustomProvider` to inject your provider

See the source code of this module as a reference implementation.
