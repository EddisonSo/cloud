.PHONY: proto build clean

PROTO_DIR = ../proto
OUT_DIR = pkg/pb

proto:
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUT_DIR) \
		--go_opt=Mcommon/types.proto=eddisonso.com/alerting-service/pkg/pb/common \
		--go_opt=Mcluster/events.proto=eddisonso.com/alerting-service/pkg/pb/cluster \
		--go_opt=Mlog/events.proto=eddisonso.com/alerting-service/pkg/pb/log \
		--go_opt=module=eddisonso.com/alerting-service/pkg/pb \
		$(PROTO_DIR)/common/types.proto \
		$(PROTO_DIR)/cluster/events.proto \
		$(PROTO_DIR)/log/events.proto

build: proto
	go build -o bin/alerting-service .

clean:
	rm -rf bin/
