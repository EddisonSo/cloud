module eddisonso.com/edd-cloud-auth

go 1.24.0

toolchain go1.24.13

require (
	eddisonso.com/go-gfs v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/lib/pq v1.10.9
	github.com/nats-io/nats.go v1.38.0
	golang.org/x/crypto v0.36.0
)

replace eddisonso.com/go-gfs => ../go-gfs

require (
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)
