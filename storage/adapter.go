package storage

import "fmt"

// Adapter identifies a storage backend.
type Adapter uint8

// Adapter backend identifiers.
const (
	AdapterLocal Adapter = iota
	AdapterS3
	AdapterR2
)

// String returns the canonical adapter name.
func (a Adapter) String() string {
	switch a {
	case AdapterLocal:
		return "local"
	case AdapterS3:
		return "s3"
	case AdapterR2:
		return "r2"
	default:
		return fmt.Sprintf("Adapter(%d)", int(a))
	}
}

// ParseAdapter maps a name to its Adapter value.
func ParseAdapter(adapter string) (Adapter, error) {
	switch adapter {
	case "local":
		return AdapterLocal, nil
	case "s3":
		return AdapterS3, nil
	case "r2":
		return AdapterR2, nil
	default:
		return AdapterLocal, &InvalidAdapterError{Adapter: adapter}
	}
}
