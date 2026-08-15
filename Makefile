BINARY := bin/cinch

.PHONY: build run test vet fmt tidy clean

build:
	go build -o $(BINARY) ./cmd/cinch

run:
	go run ./cmd/cinch

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
