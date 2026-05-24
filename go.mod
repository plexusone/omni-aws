module github.com/plexusone/omni-aws

go 1.26.0

require (
	// omnistorage dependencies
	github.com/aws/aws-sdk-go-v2 v1.41.7
	// omnillm dependencies
	github.com/aws/aws-sdk-go-v2/config v1.32.18
	github.com/aws/aws-sdk-go-v2/credentials v1.19.17
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.1.22
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.52.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.101.0
	// omnivault dependencies
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.41.7
	github.com/aws/aws-sdk-go-v2/service/ssm v1.68.6
	github.com/aws/smithy-go v1.25.1
	github.com/grokify/mogo v0.74.5
	github.com/plexusone/omnillm-core v0.16.0
	github.com/plexusone/omnistorage-core v0.4.0
	github.com/plexusone/omnivault v0.5.0
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.10 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.1 // indirect
	github.com/grokify/oscompat v0.3.0 // indirect
)
