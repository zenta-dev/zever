package middleware

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/shared/codec"
)

// errorBody is the fixed-shape JSON error envelope ({"error": "<message>"})
// recovery and rate-limit denials return. The client never sees the panic
// value: internal detail stays in the logs.
type errorBody struct {
	Error string `json:"error"`
}

// errorBodyCodec encodes errorBody for HTTP error responses. Shared by
// Recover and RateLimit (both in package middleware).
var errorBodyCodec = codec.JSONCodec[errorBody]{}

// Recover returns HTTP middleware that recovers a panicking handler, logs the
// panic value with a stack trace via logger, and responds 500 with the fixed
// error shape -- instead of the panic crashing the whole process, which is
// what happens with no recovery middleware in the request path.
//
// Recovery must run outermost (first in the chain) so it also catches panics
// from inner middleware. The panic value and stack go to LOGS ONLY; the
// client gets the fixed "internal error" message.
func Recover(logger log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := newStatusRecorder(w)
			defer func() {
				if p := recover(); p != nil {
					logger.Error().
						Str("panic", fmt.Sprint(p)).
						Str("stack", string(debug.Stack())).
						Msg("panic recovered")

					if !rec.wroteHeader {
						rec.Header().Set("Content-Type", "application/json")
						rec.WriteHeader(http.StatusInternalServerError)
						// Best-effort: headers + status are already on the
						// wire, so a failed body encode still leaves a
						// valid empty 500 instead of crashing recovery.
						if data, encErr := errorBodyCodec.Encode(errorBody{Error: "internal error"}); encErr == nil {
							_, _ = rec.Write(append(data, '\n'))
						}
					}
					// Else the handler already committed the response:
					// another WriteHeader would be superfluous and a fresh
					// JSON body would truncate/corrupt the stream, so log
					// only and leave the committed bytes untouched.
				}
			}()

			next.ServeHTTP(rec, r)
		})
	}
}

// RecoverUnaryServerInterceptor is Recover's gRPC counterpart: it recovers a
// panicking handler, logs it, and returns codes.Internal instead of letting
// the panic crash the process. Place recovery last in the interceptor chain
// so a panic cannot skip the other interceptors.
func RecoverUnaryServerInterceptor(logger log.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if p := recover(); p != nil {
				logger.Error().
					Str("panic", fmt.Sprint(p)).
					Str("stack", string(debug.Stack())).
					Str("method", info.FullMethod).
					Msg("panic recovered")

				err = status.Error(codes.Internal, "internal error")
			}
		}()

		return handler(ctx, req)
	}
}
