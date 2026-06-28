// Package omnimemory provides AWS-based providers for omnimemory.
//
// This package contains multiple AWS service implementations of the
// omnimemory Provider interface:
//
//   - dynamodb: DynamoDB-based storage with in-memory vector search
//   - s3: S3-based storage for simple use cases (planned)
//   - opensearch: OpenSearch with k-NN for native vector search (planned)
//
// # DynamoDB Provider
//
// The DynamoDB provider stores memories in a DynamoDB table with the
// following schema:
//
//   - Partition Key (pk): tenant_id
//   - Sort Key (sk): subject_id#memory_id
//   - GSI1: type-index for filtering by memory type
//   - GSI2: scope-index for filtering by memory scope
//
// DynamoDB supports automatic TTL-based expiration via the expires_at
// attribute.
//
// # Usage
//
//	import (
//	    "github.com/plexusone/omnimemory"
//	    "github.com/plexusone/omnimemory/core"
//	    _ "github.com/plexusone/omni-aws/omnimemory/dynamodb"
//	)
//
//	func main() {
//	    client, err := omnimemory.NewClient(core.ClientConfig{
//	        Providers: []core.ProviderConfig{
//	            {
//	                Name: core.ProviderNameAWSDynamoDB,
//	                Options: map[string]any{
//	                    "table_name": "omnimemory",
//	                    "region":     "us-east-1",
//	                },
//	            },
//	        },
//	    })
//	    // ...
//	}
package omnimemory
