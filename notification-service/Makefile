.PHONY: proto build clean

PROTO_DIR = ../proto
OUT_DIR = pkg/pb

proto:
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUT_DIR) \
		--go_opt=Mcommon/types.proto=eddisonso.com/notification-service/pkg/pb/common \
		--go_opt=Mnotification/events.proto=eddisonso.com/notification-service/pkg/pb/notification \
		--go_opt=module=eddisonso.com/notification-service/pkg/pb \
		$(PROTO_DIR)/common/types.proto \
		$(PROTO_DIR)/notification/events.proto

build: proto
	go build -o bin/notification ./cmd/notification

clean:
	rm -rf bin/
