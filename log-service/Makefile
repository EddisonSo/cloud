.PHONY: proto proto-events build clean

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/logging/logging.proto

proto-events:
	mkdir -p pkg/pb
	protoc \
		--proto_path=../proto \
		--go_out=pkg/pb \
		--go_opt=Mcommon/types.proto=github.com/eddisonso/log-service/pkg/pb/common \
		--go_opt=Mlog/events.proto=github.com/eddisonso/log-service/pkg/pb/log \
		--go_opt=module=github.com/eddisonso/log-service/pkg/pb \
		../proto/common/types.proto \
		../proto/log/events.proto

build: proto
	go build -o log-service .

clean:
	rm -f log-service
