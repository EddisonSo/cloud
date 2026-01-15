FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

COPY go-gfs /go-gfs
COPY edd-cloud-interface/services/compute /src/

RUN sed -i 's|replace eddisonso.com/go-gfs => ../../../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod
RUN go mod download
RUN CGO_ENABLED=0 go build -o /out/compute .

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/compute /app/compute

EXPOSE 8080
ENTRYPOINT ["/app/compute"]
