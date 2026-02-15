FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

COPY go-gfs /go-gfs

COPY alerting-service/go.mod alerting-service/go.sum /src/
COPY alerting-service /src/

RUN sed -i 's|replace eddisonso.com/go-gfs => ../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod

RUN CGO_ENABLED=0 go build -o /out/alerting-service .

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /out/alerting-service .
ENTRYPOINT ["./alerting-service"]
