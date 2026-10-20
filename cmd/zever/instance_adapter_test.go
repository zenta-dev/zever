package main

import (
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestInstanceAdapterKey pins the env key the named instance's adapter is
// read from: SERVICE__NAME__ADAPTER (two underscores around the instance
// name, like every other named-instance key). The default instance's
// adapter is SERVICE_ADAPTER.
func TestInstanceAdapterKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  config.Env
		svc  string
		inst string
		def  string
		want string
	}{
		{
			name: "named instance adapter",
			env:  config.Env{"DB__REPORTING__ADAPTER": "postgres"},
			svc:  "db", inst: "reporting", def: "sqlite",
			want: "postgres",
		},
		{
			name: "named instance adapter is case insensitive",
			env:  config.Env{"db__reporting__adapter": "postgres"},
			svc:  "db", inst: "reporting", def: "sqlite",
			want: "postgres",
		},
		{
			name: "default instance keeps the resolved default",
			env:  config.Env{"DB_ADAPTER": "postgres"},
			svc:  "db", inst: "", def: "sqlite",
			want: "sqlite",
		},
		{
			name: "named instance falls back to the default adapter",
			env:  config.Env{"DB_ADAPTER": "postgres"},
			svc:  "db", inst: "reporting", def: "postgres",
			want: "postgres",
		},
		{
			name: "an empty named adapter falls back to the default",
			env:  config.Env{"DB__REPORTING__ADAPTER": "  "},
			svc:  "db", inst: "reporting", def: "sqlite",
			want: "sqlite",
		},
		{
			name: "no adapter anywhere keeps the default",
			env:  config.Env{},
			svc:  "db", inst: "reporting", def: "sqlite",
			want: "sqlite",
		},
		{
			name: "the single-underscore spelling is not an instance key",
			env:  config.Env{"DB_REPORTING_ADAPTER": "postgres"},
			svc:  "db", inst: "reporting", def: "sqlite",
			want: "sqlite",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := instanceAdapter(tc.env, tc.svc, tc.inst, tc.def); got != tc.want {
				t.Errorf("instanceAdapter = %q, want %q", got, tc.want)
			}
		})
	}
}
