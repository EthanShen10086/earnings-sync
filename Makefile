.PHONY: test run tidy

test:
	go test ./...

tidy:
	go mod tidy

run:
	go run ./cmd/server -conf trpc_go.yaml
