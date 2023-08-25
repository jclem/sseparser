.PHONY: test lint check

test:
	go test -race -cover ./...

lint:
	golangci-lint run

check: test lint