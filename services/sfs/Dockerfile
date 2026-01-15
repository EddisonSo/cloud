FROM golang:1.24.11 AS builder

ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0
WORKDIR /src

COPY go-gfs /go-gfs
COPY edd-cloud-interface/services/sfs/go.mod edd-cloud-interface/services/sfs/go.sum /src/
COPY edd-cloud-interface/services/sfs/main.go /src/

RUN go mod download && go build -o /out/sfs .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/sfs /app/sfs

EXPOSE 8080
CMD ["/app/sfs"]
