.PHONY: proto

PROTO_DIR = ../proto
OUT_DIR = pkg/pb

proto:
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUT_DIR) \
		--go_opt=Mcommon/types.proto=eddisonso.com/cluster-monitor/pkg/pb/common \
		--go_opt=Mcluster/events.proto=eddisonso.com/cluster-monitor/pkg/pb/cluster \
		--go_opt=module=eddisonso.com/cluster-monitor/pkg/pb \
		$(PROTO_DIR)/common/types.proto \
		$(PROTO_DIR)/cluster/events.proto
