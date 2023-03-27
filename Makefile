.PHONY: test
test:
	go test -v ./...

.PHONY: coverage
coverage:
	go test -v ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
