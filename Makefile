.PHONY: build examples test clean

build:
	go build

examples:
	mkdir -p ./bin/
	go build -o ./bin/example ./examples/main.go

test:
	go test ./...

clean:
	rm -rf ./bin/
