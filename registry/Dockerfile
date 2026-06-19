FROM golang:1.24.11 AS builder

ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler make && rm -rf /var/lib/apt/lists/*
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

COPY go-gfs /go-gfs
COPY proto /proto
COPY notification-service /notification-service
COPY registry/go.mod registry/go.sum /src/
COPY registry/*.go /src/

# Generate notification protobuf
WORKDIR /notification-service
RUN make proto

WORKDIR /src
# go.mod replace dirs (../go-gfs, ../notification-service) resolve from /src to the
# root-level deps copied above, identically to the local layout.
RUN go mod download && go build -o /out/registry .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/registry /app/registry

EXPOSE 8080
ENTRYPOINT ["/app/registry"]
