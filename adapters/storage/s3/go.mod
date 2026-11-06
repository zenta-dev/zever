module github.com/zenta-dev/zever/adapters/storage/s3

go 1.27.3

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.25 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.4 // indirect
	github.com/aws/smithy-go v1.28.7 // indirect
	github.com/zenta-dev/zever/core/log v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/shared/s3opts => ../../../shared/s3opts

require (
	github.com/aws/aws-sdk-go-v2 v1.47.5
	github.com/aws/aws-sdk-go-v2/config v1.33.9
	github.com/aws/aws-sdk-go-v2/credentials v1.20.9
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.6
	github.com/zenta-dev/zever/core/storage v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
	github.com/zenta-dev/zever/shared/s3opts v0.8.0
)

replace github.com/zenta-dev/zever/core/storage => ../../../core/storage

replace github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop

replace github.com/zenta-dev/zever/adapters/storage/local => ../local

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
