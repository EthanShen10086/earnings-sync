.PHONY: test run tidy doctor live

test:
	go test ./...

tidy:
	go mod tidy

run:
	go run ./cmd/server -conf trpc_go.yaml

doctor:
	go run ./cmd/doctor

live:
	EARNINGS_LIVE=1 go test ./internal/live -count=1
