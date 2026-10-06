package cdn

import (
	"errors"
	"fmt"
)

var (
	ErrNilFactory       = errors.New("cdn: nil factory")
	ErrDuplicateAdapter = errors.New("cdn: duplicate adapter")
	ErrUnknownAdapter   = errors.New("cdn: unknown adapter (forgotten import?)")
	ErrInvalidAdapter   = errors.New("cdn: invalid adapter")
	ErrClosed           = errors.New("cdn: closed")
)

type InvalidAdapterError struct {
	Adapter string
}

func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("cdn: invalid adapter %q", e.Adapter)
}

func (e InvalidAdapterError) Unwrap() error {
	return ErrInvalidAdapter
}

type DuplicateError struct {
	Adapter Adapter
}

func (e DuplicateError) Error() string {
	return fmt.Sprintf("cdn: duplicate adapter %q", e.Adapter)
}

func (e DuplicateError) Unwrap() error {
	return ErrDuplicateAdapter
}

type UnknownAdapterError struct {
	Adapter Adapter
}

func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("cdn: unknown adapter %q", e.Adapter)
}

func (e UnknownAdapterError) Unwrap() error {
	return ErrUnknownAdapter
}
