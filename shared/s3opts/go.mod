module github.com/zenta-dev/zever/shared/s3opts

go 1.27.3

require (
	github.com/aws/aws-sdk-go-v2 v1.47.4
	github.com/aws/aws-sdk-go-v2/config v1.33.10
	github.com/aws/aws-sdk-go-v2/credentials v1.20.10
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.3
	github.com/zenta-dev/zever/core/storage v0.8.0
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.5 // indirect
	github.com/aws/smithy-go v1.28.6 // indirect
	github.com/zenta-dev/zever/core/log v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/codec v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/storage => ../../core/storage

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/storage/local => ../../adapters/storage/local

replace github.com/zenta-dev/zever/core/log => ../../core/log

replace github.com/zenta-dev/zever/shared/registry => ../registry

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
