package cdn

import "fmt"

type Adapter string

const (
	AdapterCloudflare Adapter = "cloudflare"
)

func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}

var _ = fmt.Sprintf
