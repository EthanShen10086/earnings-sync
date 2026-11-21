.PHONY: test fmt fmt-check run tidy doctor live hooks orchestrate orchestrate-live orchestrate-check pg-up pg-test web infra-up

test:
	go test ./...

pg-up:
	docker compose -f deploy/docker-compose.yml up -d postgres

# Optional Redis (+ NATS via mq profile). App degrades if these are down.
infra-up:
	docker compose -f deploy/docker-compose.yml --profile cache --profile mq up -d

pg-test:
	@test -n "$$DATABASE_URL" || (echo 'set DATABASE_URL first'; exit 1)
	go test ./internal/store/postgres ./internal/sheet ./internal/job ./internal/orchestrator -count=1

web:
	cd apps/web && python3 -m http.server 5173 --bind 127.0.0.1

orchestrate-check:
	go run ./cmd/orchestrate -check

orchestrate:
	go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml \
		-input ticker=AAPL -input dry_run=true -input body_path=testdata/aapl_2026q4.csv -input force=true

orchestrate-live:
	go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml \
		-input ticker=AAPL -input dry_run=false -input body_path=testdata/aapl_2026q4.csv -input force=true \
		-dump-writes data/orchestrator-e2e-writes.json

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
