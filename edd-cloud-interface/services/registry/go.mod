module eddisonso.com/edd-cloud-registry

go 1.24.0

require (
	eddisonso.com/go-gfs v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.10.9
)

require (
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace eddisonso.com/go-gfs => ../../../go-gfs
