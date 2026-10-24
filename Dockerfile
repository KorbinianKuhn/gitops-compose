# Build

FROM golang:1.26.10-alpine AS builder

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o /build ./cmd/main.go

# Runtime

FROM alpine:latest

WORKDIR /gitops-compose

COPY --from=builder /build /app

ENV IS_RUNNING_IN_DOCKER=true

EXPOSE 2112

ENTRYPOINT ["/app"]
