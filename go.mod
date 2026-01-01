module config-server

go 1.25.3

require (
	flexible-logger v0.0.0-00010101000000-000000000000
	github.com/Bastien-Antigravity/distributed-config v0.2.0
	github.com/Bastien-Antigravity/flexible-logger v0.0.0-20260101101957-fd107662dff0
	github.com/Bastien-Antigravity/safe-socket v1.3.0
	google.golang.org/protobuf v1.36.11
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.2 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	github.com/edsrzf/mmap-go v1.2.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace flexible-logger => ./pkg/flexible-logger
