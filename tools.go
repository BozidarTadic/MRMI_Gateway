//go:build tools

// tools.go pins the protobuf generator versions to the Go module graph
// so `go install` reproducibly fetches the correct versions.
//
// Usage: go generate ./proto/...  (or: make proto)
package tools

import (
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
