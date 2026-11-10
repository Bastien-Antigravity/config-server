package grpc_control

import _ "embed"

// ConfigServerProtoSchema embeds the canonical protobuf contract compiled into config-server.
//
//go:embed config_server.proto
var ConfigServerProtoSchema string

