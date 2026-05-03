// Example usage of omni-aws/omnivault
//
// This example demonstrates:
//   - Direct provider usage with Secrets Manager
//   - Convenience functions for quick setup
//   - OmniVault client integration
//
// Usage:
//
//	export AWS_REGION="us-east-1"
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/grokify/mogo/log/slogutil"
	"github.com/plexusone/omnivault"
	"github.com/plexusone/omnivault/vault"

	aws "github.com/plexusone/omni-aws/omnivault"
	"github.com/plexusone/omni-aws/omnivault/secretsmanager"
)

func main() {
	// Create base context with logger
	ctx := slogutil.ContextWithLogger(context.Background(), slog.Default())

	// Example 1: Using Secrets Manager directly
	fmt.Println("=== AWS Secrets Manager ===")
	if err := runSecretsManagerExample(ctx); err != nil {
		logError(ctx, "Secrets Manager example failed", err)
		fmt.Println("(This is expected if AWS credentials are not configured)")
	}

	// Example 2: Using convenience function with OmniVault client
	fmt.Println("\n=== Using Convenience Functions ===")
	if err := runConvenienceExample(ctx); err != nil {
		logError(ctx, "Convenience example failed", err)
	}

	// Example 3: Multi-provider resolver info
	fmt.Println("\n=== Multi-Provider Resolver ===")
	fmt.Println("In production, you could use:")
	fmt.Println("  resolver.Register(\"secret\", awsProvider)  // EKS")
	fmt.Println("  resolver.Register(\"secret\", keyringProvider)  // Local dev")
	fmt.Println("Then use: resolver.Resolve(ctx, \"secret://database/password\")")
}

// runSecretsManagerExample demonstrates direct Secrets Manager usage.
func runSecretsManagerExample(ctx context.Context) error {
	smProvider, err := secretsmanager.New(secretsmanager.Config{
		Region: getEnvOrDefault("AWS_REGION", "us-east-1"),
	})
	if err != nil {
		return fmt.Errorf("creating provider: %w", err)
	}

	// Store a secret
	if err := smProvider.Set(ctx, "myapp/test-secret", &vault.Secret{
		Value: "my-secret-value",
		Fields: map[string]string{
			"username": "admin",
			"password": "secret123",
		},
		Metadata: vault.Metadata{
			Tags: map[string]string{
				"environment": "development",
			},
		},
	}); err != nil {
		return fmt.Errorf("storing secret: %w", err)
	}
	fmt.Println("Stored secret: myapp/test-secret")

	// Retrieve the secret
	secret, err := smProvider.Get(ctx, "myapp/test-secret")
	if err != nil {
		return fmt.Errorf("getting secret: %w", err)
	}
	fmt.Printf("Retrieved: %s\n", secret.Value)
	fmt.Printf("Username: %s\n", secret.Fields["username"])

	// Clean up
	if err := smProvider.Delete(ctx, "myapp/test-secret"); err != nil {
		logError(ctx, "Failed to delete test secret", err)
	} else {
		fmt.Println("Deleted test secret")
	}

	return nil
}

// runConvenienceExample demonstrates convenience functions with OmniVault client.
func runConvenienceExample(ctx context.Context) error {
	provider, err := aws.NewSecretsManager(aws.Config{
		Region: "us-east-1",
	})
	if err != nil {
		return fmt.Errorf("creating provider: %w", err)
	}
	fmt.Printf("Created provider: %s\n", provider.Name())

	// With OmniVault client
	fmt.Println("\n=== With OmniVault Client ===")
	client, err := omnivault.NewClient(omnivault.Config{
		CustomVault: provider,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	defer func() { _ = client.Close() }()

	fmt.Printf("OmniVault client using: %s\n", client.Name())
	fmt.Printf("Capabilities: %+v\n", client.Capabilities())

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// logError logs an error message using the logger from context.
func logError(ctx context.Context, msg string, err error, args ...any) {
	logger := slogutil.LoggerFromContext(ctx, slogutil.Null())
	if err != nil {
		args = append([]any{"error", err}, args...)
	}
	logger.Error(msg, args...)
}
