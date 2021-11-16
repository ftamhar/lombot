.PHONY: build-linux,build-osx,build-win64,build-win32

build-lin:
	@GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o build/lombot-linux lombot.go
	@echo "[OK] Files build to linux"

build-osx:
	@GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o build/lombot-osx lombot.go
	@echo "[OK] Files build to OSX"

build-win64:
	@GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o build/lombot-64.exe lombot.go
	@echo "[OK] Files build to windows(64bit)"

build-win32:
	@GOOS=windows GOARCH=386 go build -ldflags "-s -w" -o build/lombot-32.exe lombot.go
	@echo "[OK] Files build to windows(32bit)"