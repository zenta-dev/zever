# Showcase production image: static binary on distroless.
# Build from repo root: docker build -f examples/showcase/Dockerfile .
# Run: docker run -p 8080:8080 -p 9090:9090 -e AUTH_JWT_SECRET=<32+bytes> showcase
FROM golang:1.27.1-bookworm AS builder
WORKDIR /src
COPY config/ ./config/
COPY container/ ./container/
COPY core/ ./core/
COPY orm/ ./orm/
COPY shared/ ./shared/
COPY dsl/ ./dsl/
COPY adapters/ ./adapters/
COPY examples/showcase/ ./examples/showcase/
# GOWORK=off: build only needs this module plus its replace targets above.
WORKDIR /src/examples/showcase
RUN GOWORK=off CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# Worker image uses the same pattern:
# RUN GOWORK=off CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=builder /out/server /server
USER nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/server"]
