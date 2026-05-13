.PHONY: test test-integration coverage build lint fmt vet clean

# Default: unit tests only (no integration build tag), with race detector.
test:
	go test -race -count=1 ./...

# Integration tests require Docker; gated by build tag.
test-integration:
	go test -race -count=1 -tags=integration ./...

# Coverage gate: combine unit + integration coverage; require ≥ 90%.
# (Pure SDK wrappers in internal/runtime/dockerclient are integration-tested
# only, so we run with -tags=integration to include them in the total.)
coverage:
	go test -race -count=1 -tags=integration -coverprofile=coverage.out -covermode=atomic ./...
	@total=$$(go tool cover -func=coverage.out | tail -n1 | awk '{print $$3}' | sed 's/%//'); \
	echo "Total coverage: $$total%"; \
	awk -v t=$$total 'BEGIN { if (t+0 < 90) { print "FAIL: coverage " t "% < 90%"; exit 1 } }'

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/worker ./cmd/worker

lint: fmt vet

fmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: the following files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

clean:
	rm -rf bin/ coverage.out coverage.html
