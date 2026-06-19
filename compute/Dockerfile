FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

RUN apk add --no-cache protobuf-dev make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

COPY go-gfs /go-gfs
COPY proto /proto
COPY notification-service /notification-service
COPY pkg /pkg
COPY compute /src/

# Generate notification protobuf
WORKDIR /notification-service
RUN make proto

WORKDIR /src
# go.mod replace dirs (../go-gfs, ../pkg/events, ../notification-service) resolve
# from /src to the root-level deps copied above, identically to the local layout.
RUN go mod download
RUN CGO_ENABLED=0 go build -o /out/compute .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/compute /app/compute

EXPOSE 8080
ENTRYPOINT ["/app/compute"]
