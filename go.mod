module github.com/plexusone/omni-aws

go 1.26.4

require (
	// omnistorage dependencies
	github.com/aws/aws-sdk-go-v2 v1.47.1
	// omnillm dependencies
	github.com/aws/aws-sdk-go-v2/config v1.33.7
	github.com/aws/aws-sdk-go-v2/credentials v1.20.7
	// omnimemory dependencies
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.21.9
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.4.14
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.63.2
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.70.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.1
	// omnivault dependencies
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.50.2
	// omnimail dependencies
	github.com/aws/aws-sdk-go-v2/service/sesv2 v1.78.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.79.1
	github.com/aws/smithy-go v1.28.4
	github.com/google/uuid v1.6.0
	github.com/grokify/mogo v0.75.0
	github.com/plexusone/omnidevx-core v0.4.0
	github.com/plexusone/omnillm-core v0.18.1
	github.com/plexusone/omnimail v0.1.0
	github.com/plexusone/omnimemory v0.2.1
	github.com/plexusone/omnistorage-core v0.5.2
	github.com/plexusone/omnivault v0.5.0
	modernc.org/sqlite v1.60.1
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.44.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.2 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/grokify/oscompat v0.5.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.48.0 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
