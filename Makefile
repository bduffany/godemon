GO_SRCS := $(shell find . -name '*.go' -type f -not -path './vendor/*' | sort -u)
GO_PKG_DIRS := $(sort $(dir $(GO_SRCS)))
GO_DEPS := go.sum $(GO_SRCS)

.PHONY: test

godemon: $(GO_DEPS)
	go build -o godemon ./cmd/godemon

test: $(GO_DEPS)
	go test ./...

.godeps: $(GO_SRCS)
	@tmp="$$(mktemp)"; \
	module="$$(awk '/^module[[:space:]]/ { print $$2; exit }' go.mod 2>/dev/null)"; \
	if [ -n "$(strip $(GO_PKG_DIRS))" ]; then \
		{ \
			go list -e -f '{{join .Imports "\n"}}{{"\n"}}{{join .TestImports "\n"}}{{"\n"}}{{join .XTestImports "\n"}}' $(GO_PKG_DIRS); \
			GOOS=darwin CGO_ENABLED=1 go list -e -f '{{join .Imports "\n"}}{{"\n"}}{{join .TestImports "\n"}}{{"\n"}}{{join .XTestImports "\n"}}' $(GO_PKG_DIRS); \
		} \
		| awk -F/ -v module="$$module" 'NF && $$1 ~ /\./ && (module == "" || ($$0 != module && index($$0, module "/") != 1))' \
		| sort -u > "$$tmp"; \
	else \
		: > "$$tmp"; \
	fi; \
	if [ ! -f $@ ] || ! cmp -s "$$tmp" $@; then \
		mv "$$tmp" $@; \
	else \
		rm -f "$$tmp"; \
	fi

go.mod go.sum &: .godeps
	go mod tidy
	touch go.mod go.sum
