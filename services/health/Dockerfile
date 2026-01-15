FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

COPY go-gfs /go-gfs
COPY edd-cloud-interface/services/health/go.mod edd-cloud-interface/services/health/go.sum /src/
COPY edd-cloud-interface/services/health/main.go /src/

RUN sed -i 's|replace eddisonso.com/go-gfs => ../../../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod
RUN CGO_ENABLED=0 go build -o /out/health .

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /out/health .
ENTRYPOINT ["./health"]
