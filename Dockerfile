# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM ghcr.io/denysvitali/base-alpine:latest AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -ldflags="-s -w" \
      -o /usr/local/bin/argocd-mcp .

FROM ghcr.io/denysvitali/base-alpine:latest

COPY --from=builder /usr/local/bin/argocd-mcp /usr/local/bin/argocd-mcp

ENTRYPOINT ["argocd-mcp", "serve"]