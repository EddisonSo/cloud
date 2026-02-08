FROM golang:1.24.11 AS builder

ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0
WORKDIR /src

RUN apk add --no-cache protobuf-dev make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

COPY go-gfs /go-gfs
COPY proto /proto
COPY notification-service /notification-service
COPY edd-cloud-interface/pkg /edd-cloud-interface/pkg
COPY edd-cloud-interface/services/sfs/go.mod edd-cloud-interface/services/sfs/go.sum /src/
COPY edd-cloud-interface/services/sfs/*.go /src/

# Generate notification protobuf
WORKDIR /notification-service
RUN make proto

WORKDIR /src
RUN sed -i 's|replace eddisonso.com/go-gfs => ../../../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod
RUN sed -i 's|replace eddisonso.com/edd-cloud/pkg/events => ../../pkg/events|replace eddisonso.com/edd-cloud/pkg/events => /edd-cloud-interface/pkg/events|' go.mod
RUN sed -i 's|replace eddisonso.com/notification-service => ../../../notification-service|replace eddisonso.com/notification-service => /notification-service|' go.mod
RUN go mod download && go build -o /out/sfs .

FROM alpine:3.20

RUN apk add --no-cache mailcap

WORKDIR /app
COPY --from=builder /out/sfs /app/sfs

EXPOSE 8080
CMD ["/app/sfs"]
