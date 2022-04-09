generate:
	go generate
.PHONY: generate

test:
	go test -v -cover .
.PHONY: test

test-yaegi:
	yaegi test -v .
.PHONY: test-yaegi