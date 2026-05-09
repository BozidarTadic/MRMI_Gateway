.PHONY: proto test build vet

proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		proto/mrmi/v1/contracts.proto

test:
	go test ./...

build:
	go build ./cmd/mrmi-gateway

vet:
	go vet ./...
