FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

RUN apk add --no-cache protobuf-dev make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

COPY go-gfs /go-gfs
COPY proto /proto
COPY notification-service /notification-service
COPY edd-cloud-interface/pkg /edd-cloud-interface/pkg
COPY edd-cloud-interface/services/compute /src/

# Generate notification protobuf
WORKDIR /notification-service
RUN make proto

WORKDIR /src
RUN sed -i 's|replace eddisonso.com/go-gfs => ../../../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod
RUN sed -i 's|replace eddisonso.com/edd-cloud/pkg/events => ../../pkg/events|replace eddisonso.com/edd-cloud/pkg/events => /edd-cloud-interface/pkg/events|' go.mod
RUN sed -i 's|replace eddisonso.com/notification-service => ../../../notification-service|replace eddisonso.com/notification-service => /notification-service|' go.mod
RUN go mod download
RUN CGO_ENABLED=0 go build -o /out/compute .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/compute /app/compute

EXPOSE 8080
ENTRYPOINT ["/app/compute"]
