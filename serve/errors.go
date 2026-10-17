package serve

import "errors"

// ErrDoctorFailed is returned by Main in doctor mode when one or more
// batteries fail to resolve. The JSON report on stdout names each failure;
// this error only sets the exit path.
var ErrDoctorFailed = errors.New("serve: doctor: one or more batteries failed to resolve (see --doctor output)")

// ErrDuplicateModule is returned when two modules share one Name.
var ErrDuplicateModule = errors.New("serve: duplicate module name")

// ErrGRPCPoliciesWithoutAddr is returned when a module declares gRPC
// policies but no gRPC listen address is configured.
var ErrGRPCPoliciesWithoutAddr = errors.New("serve: module declares gRPC policies but no gRPC address is set (set GRPC_ADDR=:9090 or WithGRPCAddr)")
