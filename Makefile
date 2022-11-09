TMP_DIR?=./tests/tmp

test: ensure-testdata
	go test ./...

test-386:
	GOARCH=386 go test ./...

bench-1-goprocs:
	GOMAXPROCS=1 go test -test.bench=".*"

bench-2-goprocs:
	GOMAXPROCS=2 go test -test.bench=".*"

bench-4-goprocs:
	GOMAXPROCS=4 go test -test.bench=".*"

bench-8-goprocs:
	GOMAXPROCS=8 go test -test.bench=".*"

fmt:
	go fmt ./...

ensure-tmp:
	mkdir -p $(TMP_DIR)

ensure-testdata: ensure-tmp
	if [ ! -f $(TMP_DIR)/testdata ]; then dd if=/dev/urandom of=$(TMP_DIR)/testdata bs=4096 count=32; fi