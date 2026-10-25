# Build

FROM golang:1.27.3-alpine3.24 AS builder

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o /build ./cmd/main.go

# Runtime

FROM alpine:3.24.3

WORKDIR /gitops-compose

COPY --from=builder /build /app

ENV IS_RUNNING_IN_DOCKER=true

EXPOSE 2112

ENTRYPOINT ["/app"]
