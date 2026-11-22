# Thin monorepo boundary — Go module stays at repo root.
ROOT := $(abspath ../..)

.PHONY: run test doctor fmt

run:
	cd $(ROOT) && $(MAKE) run

test:
	cd $(ROOT) && $(MAKE) test

doctor:
	cd $(ROOT) && $(MAKE) doctor

fmt:
	cd $(ROOT) && $(MAKE) fmt
