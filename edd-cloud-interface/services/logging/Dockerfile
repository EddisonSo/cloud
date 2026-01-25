FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

COPY go-gfs /go-gfs
COPY edd-cloud-interface/services/logging /src/

RUN sed -i 's|replace eddisonso.com/go-gfs => ../../../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod
RUN CGO_ENABLED=0 go build -o /out/logging .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/logging /app/logging

EXPOSE 50051 8080
ENTRYPOINT ["/app/logging"]
