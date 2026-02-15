module eddisonso.com/alerting-service

go 1.24.0

toolchain go1.24.11

require (
	eddisonso.com/go-gfs v0.0.0
	github.com/nats-io/nats.go v1.48.0
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	google.golang.org/grpc v1.73.0 // indirect
)

replace eddisonso.com/go-gfs => ../go-gfs
