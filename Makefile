all: build

build:
	mkdir -p bin
	go build -o bin/nodemcu-workbench

test:
	go test ./...

clean:
	rm -f bin/*
