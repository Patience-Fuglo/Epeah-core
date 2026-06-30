# ==========================================
# STAGE 1: Build the Rust Daemon
# ==========================================
FROM rust:1.77-slim AS rust-builder
WORKDIR /usr/src/arbiter-rust
RUN apt-get update && apt-get install -y pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*
COPY ./shadow-engine .
RUN cargo build --release

# ==========================================
# STAGE 2: Build the Go Microservice
# ==========================================
FROM golang:1.22-alpine AS go-builder
WORKDIR /go/src/arbiter-go
COPY go.mod ./
COPY main.go enclave_stub.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# ==========================================
# STAGE 3: Production High-Performance Runtime
# ==========================================
FROM alpine:3.19
WORKDIR /app

RUN apk --no-cache add ca-certificates

COPY --from=rust-builder /usr/src/arbiter-rust/target/release/arbiter-shadow-engine ./arbiter-daemon
COPY --from=go-builder /go/src/arbiter-go/main ./arbiter-gateway
COPY config.yaml ./config.yaml
COPY web/ ./web/

RUN mkdir -p /var/run/arbiter /var/log/arbiter

ENV ARBITER_ENV=DEV

EXPOSE 8080

RUN echo '#!/bin/sh' > /app/entrypoint.sh && \
    echo './arbiter-daemon &' >> /app/entrypoint.sh && \
    echo 'sleep 1' >> /app/entrypoint.sh && \
    echo './arbiter-gateway' >> /app/entrypoint.sh && \
    chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
