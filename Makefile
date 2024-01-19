# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

all:
	go mod tidy
	go build -o ./build/blow ./cmd/blow 
	go build -o ./build/echo ./cmd/echo
	go build -o ./build/beat ./cmd/beat
	go build -o ./build/kite ./cmd/kite

kite:
	go mod tidy
	go build -o ./build/kite ./cmd/kite
	