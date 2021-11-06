generate:
	go generate
.PHONY: generate

test:
	go test -v -cover ./...
.PHONY: test