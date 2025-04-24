# Build

FROM golang:1.24.2-alpine AS builder

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o /build ./cmd/main.go

# Docker CLI

FROM docker:cli AS docker-cli

RUN apk add --no-cache curl
RUN mkdir -p /usr/local/lib/docker/cli-plugins
RUN curl -L "https://github.com/docker/compose/releases/download/v2.24.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/lib/docker/cli-plugins/docker-compose \
    && chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

# Runtime

FROM alpine:latest

WORKDIR /gitops-compose

COPY --from=builder /build /app

COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker-cli /usr/local/lib/docker/cli-plugins/docker-compose /usr/local/lib/docker/cli-plugins/docker-compose

EXPOSE 2112

ENTRYPOINT ["/app"]
