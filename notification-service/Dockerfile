FROM golang:1.24-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git protobuf-dev make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Copy go-gfs dependency
COPY go-gfs/ ./go-gfs/

# Copy proto definitions
COPY proto/ ./proto/

# Copy notification service source
COPY notification-service/ ./notification-service/

WORKDIR /build/notification-service

# Generate protobuf code
RUN make proto

# Download dependencies and build
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /notification ./cmd/notification

# Final stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /notification /app/notification

EXPOSE 8080

ENTRYPOINT ["/app/notification"]
