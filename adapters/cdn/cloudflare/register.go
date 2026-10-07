package cloudflare

import "github.com/zenta-dev/zever/core/cdn"

func Register() {
	_ = cdn.Register(cdn.AdapterCloudflare, New)
}
