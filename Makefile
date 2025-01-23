.PHONY: build

build: build-linux-amd64 build-linux-arm64 build-darwin-arm64

build-darwin-arm64:
	@rm -rf build || true
	@mkdir -p build || true
	@go mod tidy
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o build/monica .

build-linux-amd64:
	@rm -rf build || true
	@mkdir -p build || true
	@go mod tidy
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o build/monica .
	@upx -7 build/monica

build-linux-arm64:
	@rm -rf build || true
	@mkdir -p build || true
	@go mod tidy
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o build/monica .
	@upx -7 build/monica