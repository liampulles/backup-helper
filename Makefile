# Init variables
GOBIN := $(shell go env GOPATH)/bin

# Keep test at the top
coverage.txt:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
view-cover: clean coverage.txt
	go tool cover -html=coverage.txt
test: build
	go test ./...
build:
	go build ./...
install: build
	go install ./...
	rm -f backup-helper
update:
	go get -u ./...
pre-commit: update clean coverage.txt
	go mod tidy
clean:
	rm -f coverage.txt $(GOBIN)/backup-helper *.log