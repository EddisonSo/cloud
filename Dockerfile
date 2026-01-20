FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth ./cmd/auth

# Final stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /auth /app/auth

EXPOSE 8080

ENTRYPOINT ["/app/auth"]
