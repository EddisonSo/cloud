FROM golang:1.24.11 AS builder

ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0
WORKDIR /src

COPY go-gfs /go-gfs
COPY edd-cloud-interface/services/registry/go.mod edd-cloud-interface/services/registry/go.sum /src/
COPY edd-cloud-interface/services/registry/*.go /src/

WORKDIR /src
RUN sed -i 's|replace eddisonso.com/go-gfs => ../../../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod
RUN go mod download && go build -o /out/registry .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/registry /app/registry

EXPOSE 8080
ENTRYPOINT ["registry"]
