# OmniVault Provider for AWS

AWS secret storage providers for [OmniVault](https://github.com/plexusone/omnivault). Supports AWS Secrets Manager and AWS Systems Manager Parameter Store.

## Installation

```bash
go get github.com/plexusone/omni-aws
```

## Features

- **AWS Secrets Manager** (`aws-sm`): Designed for credentials, API keys, and rotating secrets
- **AWS Parameter Store** (`aws-ssm`): Designed for configuration and hierarchical parameters
- **IRSA Support**: Automatic authentication on EKS with IAM Roles for Service Accounts
- **Multi-Field Secrets**: JSON parsing for complex secrets with username, password, etc.
- **Versioning**: Access specific versions of secrets
- **Secret Rotation**: Trigger rotation for Secrets Manager secrets

## Quick Start

### AWS Secrets Manager

```go
package main

import (
    "context"
    "fmt"
    "log"

    aws "github.com/plexusone/omni-aws/omnivault"
)

func main() {
    ctx := context.Background()

    // Create Secrets Manager provider
    provider, err := aws.NewSecretsManager(aws.Config{
        Region: "us-east-1",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Get a secret
    secret, err := provider.Get(ctx, "prod/database/credentials")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Password:", secret.Value)
    fmt.Println("Username:", secret.Fields["username"])
}
```

### AWS Parameter Store

```go
package main

import (
    "context"
    "fmt"
    "log"

    aws "github.com/plexusone/omni-aws/omnivault"
)

func main() {
    ctx := context.Background()

    // Create Parameter Store provider
    provider, err := aws.NewParameterStore(aws.Config{
        Region: "us-east-1",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Get a parameter
    secret, err := provider.Get(ctx, "/myapp/prod/database/password")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Password:", secret.Value)
}
```

## Provider Comparison

| Feature | Secrets Manager (`aws-sm`) | Parameter Store (`aws-ssm`) |
|---------|---------------------------|----------------------------|
| **Best For** | Credentials, API keys | Configuration, feature flags |
| **Rotation** | Built-in rotation support | Manual only |
| **Pricing** | $0.40/secret/month + API calls | Free tier: 10K params, then $0.05/10K |
| **Size Limit** | 64 KB | 4 KB (standard), 8 KB (advanced) |
| **Versioning** | Automatic | Automatic |
| **Hierarchy** | Flat names | Path-based (`/app/env/key`) |
| **Cross-Account** | Resource policies | Resource policies |

## EKS with IRSA (IAM Roles for Service Accounts)

On EKS, authentication is automatic when using IRSA. No credentials configuration needed.

### 1. Create IAM Policy

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "secretsmanager:GetSecretValue",
                "secretsmanager:DescribeSecret"
            ],
            "Resource": "arn:aws:secretsmanager:us-east-1:123456789:secret:myapp/*"
        }
    ]
}
```

### 2. Create Service Account with IAM Role

```bash
eksctl create iamserviceaccount \
    --name myapp-sa \
    --namespace default \
    --cluster my-cluster \
    --attach-policy-arn arn:aws:iam::123456789:policy/MyAppSecretsPolicy \
    --approve
```

### 3. Use in Pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
spec:
  serviceAccountName: myapp-sa  # IRSA-enabled service account
  containers:
  - name: app
    image: myapp:latest
    env:
    - name: AWS_REGION
      value: "us-east-1"
```

### 4. Application Code

```go
// On EKS with IRSA, credentials are automatic
provider, err := aws.NewSecretsManager(aws.Config{
    Region: os.Getenv("AWS_REGION"),
})
// That's it! IRSA handles authentication
```

## Configuration

### Basic Config

```go
type Config struct {
    // Region is the AWS region (e.g., "us-east-1")
    Region string

    // Profile is the AWS credentials profile name
    Profile string

    // EndpointURL is for LocalStack or testing
    EndpointURL string

    // AWSConfig is an optional pre-configured AWS SDK config
    AWSConfig *aws.Config
}
```

### Secrets Manager Config

```go
import "github.com/plexusone/omni-aws/omnivault/secretsmanager"

provider, err := secretsmanager.New(secretsmanager.Config{
    Region:       "us-east-1",
    Profile:      "dev",           // Optional: AWS profile
    JSONParse:    aws.Bool(true),  // Parse JSON secrets into Fields
    VersionStage: "AWSCURRENT",    // Version stage to retrieve
})
```

### Parameter Store Config

```go
import "github.com/plexusone/omni-aws/omnivault/parameterstore"

provider, err := parameterstore.New(parameterstore.Config{
    Region:         "us-east-1",
    Profile:        "dev",
    PathPrefix:     "/myapp/prod",     // Prefix for all paths
    WithDecryption: aws.Bool(true),    // Decrypt SecureString
    DefaultType:    types.ParameterTypeSecureString,
    KMSKeyID:       "alias/myapp-key", // Custom KMS key
})
```

## Usage Examples

### Store and Retrieve Multi-Field Secrets

```go
// Store a database credential with multiple fields
err := provider.Set(ctx, "database/prod", &vault.Secret{
    Fields: map[string]string{
        "username": "admin",
        "password": "super-secret",
        "host":     "db.example.com",
        "port":     "5432",
    },
    Metadata: vault.Metadata{
        Tags: map[string]string{
            "environment": "production",
            "team":        "backend",
        },
    },
})

// Retrieve and access fields
secret, _ := provider.Get(ctx, "database/prod")
fmt.Printf("postgres://%s:%s@%s:%s/mydb",
    secret.Fields["username"],
    secret.Fields["password"],
    secret.Fields["host"],
    secret.Fields["port"],
)
```

### List Secrets

```go
// List all secrets with prefix
secrets, err := provider.List(ctx, "myapp/")
for _, name := range secrets {
    fmt.Println(name)
}
```

### Version Management

```go
import "github.com/plexusone/omni-aws/omnivault/secretsmanager"

provider, _ := secretsmanager.New(config)

// List all versions
versions, _ := provider.ListVersions(ctx, "my-secret")
for _, v := range versions {
    fmt.Printf("Version: %s, Current: %t\n", v.ID, v.Current)
}

// Get specific version
secret, _ := provider.GetVersion(ctx, "my-secret", "abc123-version-id")
```

### Secret Rotation

```go
import "github.com/plexusone/omni-aws/omnivault/secretsmanager"

provider, _ := secretsmanager.New(config)

// Trigger rotation (requires rotation Lambda configured in AWS)
newSecret, err := provider.Rotate(ctx, "my-secret")
if err != nil {
    log.Fatal(err)
}
fmt.Println("New secret value:", newSecret.Value)
```

### With OmniVault Client

```go
import (
    "github.com/plexusone/omnivault"
    aws "github.com/plexusone/omni-aws/omnivault"
)

provider, _ := aws.NewSecretsManager(aws.Config{Region: "us-east-1"})

client, _ := omnivault.NewClient(omnivault.Config{
    CustomVault: provider,
})
defer client.Close()

// Use standard OmniVault API
password, _ := client.GetValue(ctx, "database/password")
username, _ := client.GetField(ctx, "database/credentials", "username")
```

### Multi-Provider Resolver

```go
resolver := omnivault.NewResolver()
resolver.Register("aws-sm", smProvider)
resolver.Register("aws-ssm", ssmProvider)

// Route to different backends via URI
dbPassword, _ := resolver.Resolve(ctx, "aws-sm://database/password")
appConfig, _ := resolver.Resolve(ctx, "aws-ssm:///myapp/config/feature-flag")
```

## Local Development

### Using AWS CLI Profiles

```bash
# Configure a profile
aws configure --profile dev

# Use in code
provider, _ := aws.NewSecretsManager(aws.Config{
    Profile: "dev",
})
```

### Using LocalStack

```go
provider, _ := aws.NewSecretsManager(aws.Config{
    EndpointURL: "http://localhost:4566",
    Region:      "us-east-1",
})
```

### Using Environment Variables

```bash
export AWS_ACCESS_KEY_ID=xxx
export AWS_SECRET_ACCESS_KEY=xxx
export AWS_REGION=us-east-1

# Or use AWS SSO
aws sso login --profile dev
export AWS_PROFILE=dev
```

## IAM Permissions

### Secrets Manager

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "secretsmanager:GetSecretValue",
                "secretsmanager:DescribeSecret",
                "secretsmanager:ListSecrets"
            ],
            "Resource": "*"
        }
    ]
}
```

For write access, add:

```json
{
    "Action": [
        "secretsmanager:CreateSecret",
        "secretsmanager:PutSecretValue",
        "secretsmanager:DeleteSecret"
    ]
}
```

### Parameter Store

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ssm:GetParameter",
                "ssm:GetParameters",
                "ssm:GetParametersByPath"
            ],
            "Resource": "arn:aws:ssm:*:*:parameter/myapp/*"
        }
    ]
}
```

For SecureString parameters, add KMS access:

```json
{
    "Effect": "Allow",
    "Action": [
        "kms:Decrypt"
    ],
    "Resource": "arn:aws:kms:*:*:key/your-kms-key-id"
}
```

## Error Handling

```go
import (
    "errors"
    "github.com/plexusone/omnivault/vault"
)

secret, err := provider.Get(ctx, "my-secret")
if err != nil {
    switch {
    case errors.Is(err, vault.ErrSecretNotFound):
        log.Println("Secret not found")
    case errors.Is(err, vault.ErrAccessDenied):
        log.Println("Access denied - check IAM permissions")
    default:
        log.Printf("Error: %v", err)
    }
}
```

## URI Schemes

| Service | Scheme | Example |
|---------|--------|---------|
| Secrets Manager | `aws-sm://` | `aws-sm://database/credentials` |
| Parameter Store | `aws-ssm://` | `aws-ssm:///myapp/prod/config` |

## Related Projects

- [OmniVault](https://github.com/plexusone/omnivault) - Core library
- [OmniVault Keyring](https://github.com/plexusone/omnivault-keyring) - OS credential stores
- [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2) - Underlying AWS SDK
