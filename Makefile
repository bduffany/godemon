.PHONY: godemon test

godemon:
	go build -o godemon ./cmd/godemon

test:
	go test ./...
