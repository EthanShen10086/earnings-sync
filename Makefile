.PHONY: test fmt fmt-check run tidy doctor live hooks orchestrate

test:
	go test ./...

orchestrate:
	go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml -input ticker=AAPL -input dry_run=true

fmt:
	gofmt -w $$(git ls-files '*.go')

fmt-check:
	@unformatted=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt would change:"; \
		printf '%s\n' "$$unformatted"; \
		exit 1; \
	fi

hooks:
	cp githooks/deny-master .git/hooks/pre-commit
	cp githooks/deny-master .git/hooks/pre-merge-commit
	cp githooks/deny-master .git/hooks/pre-push
	chmod +x .git/hooks/pre-commit .git/hooks/pre-merge-commit .git/hooks/pre-push

tidy:
	go mod tidy

run:
	go run ./cmd/server -conf trpc_go.yaml

doctor:
	go run ./cmd/doctor

live:
	EARNINGS_LIVE=1 go test ./internal/live -count=1
