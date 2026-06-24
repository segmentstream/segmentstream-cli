# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS builder

WORKDIR /src/cli
COPY cli/go.mod cli/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cli/ ./

ARG SEGMENTSTREAM_VERSION=dev
ARG SEGMENTSTREAM_COMMIT=none
ARG SEGMENTSTREAM_DATE=unknown

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w \
        -X github.com/segmentstream/segmentstream-cli/cli/internal/version.Version=${SEGMENTSTREAM_VERSION} \
        -X github.com/segmentstream/segmentstream-cli/cli/internal/version.Commit=${SEGMENTSTREAM_COMMIT} \
        -X github.com/segmentstream/segmentstream-cli/cli/internal/version.Date=${SEGMENTSTREAM_DATE}" \
      -o /out/segmentstream ./cmd/segmentstream

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/segmentstream /usr/local/bin/segmentstream

WORKDIR /workspace
