.PHONY: build-linux,build-osx

build-linux:
	@GOOS=linux GOARCH=amd64 go build -o build/lombot-linux lombot.go
	@echo "[OK] Files build to linux"

build-osx:
	@GOOS=darwin GOARCH=amd64 go build -o build/lombot-osx lombot.go
	@echo "[OK] Files build to OSX"