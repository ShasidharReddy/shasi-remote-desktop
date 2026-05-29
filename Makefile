.PHONY: build dist build-mac build-win build-linux clean bundle-mac help

BINARY  := shasi-remote-desktop
VERSION ?= 1.0.0
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"
WIN_LDF := -ldflags "-s -w -H windowsgui -X main.Version=$(VERSION)"

help:
	@printf "\n  Secure System  —  Build Commands\n\n"
	@printf "  make build        Build for current OS\n"
	@printf "  make dist         Build for ALL platforms (mac/win/linux)\n"
	@printf "  make bundle-mac   Create macOS .app bundle\n"
	@printf "  make clean        Remove dist/\n\n"

build:
	@go build $(LDFLAGS) -o $(BINARY) .
	@echo "Built: ./$(BINARY)"

dist: clean build-mac build-win build-linux
	@echo ""; ls -lh dist/

build-mac:
	@mkdir -p dist
	@GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o "dist/SecureSystem-mac-arm64"      .
	@GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o "dist/SecureSystem-mac-intel"     .
	@echo "macOS: dist/SecureSystem-mac-arm64 (Apple Silicon)"
	@echo "macOS: dist/SecureSystem-mac-intel (Intel)"

build-win:
	@mkdir -p dist
	@GOOS=windows GOARCH=amd64 go build $(WIN_LDF) -o "dist/SecureSystem-windows-x64.exe" .
	@echo "Windows: dist/SecureSystem-windows-x64.exe"

build-linux:
	@mkdir -p dist
	@GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o "dist/SecureSystem-linux-x64"      .
	@GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o "dist/SecureSystem-linux-arm64"    .
	@echo "Linux: dist/SecureSystem-linux-x64"
	@echo "Linux: dist/SecureSystem-linux-arm64"

bundle-mac: build-mac
	@mkdir -p "dist/Secure System.app/Contents/MacOS"
	@cp "dist/SecureSystem-mac-arm64" "dist/Secure System.app/Contents/MacOS/Secure System"
	@chmod +x "dist/Secure System.app/Contents/MacOS/Secure System"
	@echo '<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>Secure System</string><key>CFBundleIdentifier</key><string>com.securesystem.remotedesktop</string><key>CFBundleName</key><string>Secure System</string><key>CFBundleVersion</key><string>1.0.0</string><key>CFBundlePackageType</key><string>APPL</string><key>NSHighResolutionCapable</key><true/></dict></plist>' > "dist/Secure System.app/Contents/Info.plist"
	@echo "Created: dist/Secure System.app"

clean:
	@rm -rf dist/ $(BINARY)
	@echo "Cleaned"
