FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install dependencies
RUN apk add --no-cache git

# Copy go-gfs dependency
COPY go-gfs/ ./go-gfs/

# Copy auth source
COPY edd-cloud-auth/ ./edd-cloud-auth/

WORKDIR /build/edd-cloud-auth

# Download dependencies and build
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth ./cmd/auth

# Final stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /auth /app/auth

EXPOSE 8080

ENTRYPOINT ["/app/auth"]
