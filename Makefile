# Build score2audio. Needs Go 1.22 or newer (on Arch Linux: sudo pacman -S go).
#
#   make            build for this computer
#   make test       run the tests
#   make run        build and start it
#   make dist       build download packages for Windows, Mac and Linux
#   make install    install for this user on Linux (into ~/.local)

VERSION ?= $(or $(shell git describe --tags --always 2>/dev/null | sed "s/^v//"),dev)
LDFLAGS := -s -w -X main.appVersion=$(VERSION)
GO      ?= go
TESSDATA_URL := https://github.com/tesseract-ocr/tessdata/raw/main/eng.traineddata
PREFIX  ?= $(HOME)/.local

.PHONY: all build test run dist clean install tessdata

all: build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o score2audio$(if $(filter windows,$(shell $(GO) env GOOS)),.exe,) .

test:
	$(GO) vet ./...
	$(GO) test ./...

run: build tessdata
	./score2audio

tessdata/eng.traineddata:
	mkdir -p tessdata
	curl -fsSL -o $@ $(TESSDATA_URL)

tessdata: tessdata/eng.traineddata

dist: test tessdata
	rm -rf build dist && mkdir -p dist
	# Windows
	mkdir -p build/windows/tessdata
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/windows/score2audio.exe .
	cp tessdata/eng.traineddata build/windows/tessdata/
	cp README.md build/windows/README.txt && cp LICENSE build/windows/LICENSE.txt
	cd build/windows && zip -qr ../../dist/score2audio-windows.zip .
	# Mac: one program for both Apple silicon and Intel
	mkdir -p build/mac/score2audio/tessdata
	GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/score2audio-mac-arm64 .
	GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/score2audio-mac-amd64 .
	$(GO) run ./tools/makefat build/mac/score2audio/score2audio build/score2audio-mac-arm64 build/score2audio-mac-amd64
	cp packaging/mac/start-score2audio.command build/mac/score2audio/
	chmod +x build/mac/score2audio/score2audio build/mac/score2audio/start-score2audio.command
	cp tessdata/eng.traineddata build/mac/score2audio/tessdata/
	cp README.md LICENSE build/mac/score2audio/
	cd build/mac && zip -qry ../../dist/score2audio-mac.zip score2audio
	# Linux
	mkdir -p build/linux/score2audio/tessdata
	GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/linux/score2audio/score2audio .
	cp tessdata/eng.traineddata build/linux/score2audio/tessdata/
	cp README.md LICENSE packaging/linux/score2audio.desktop build/linux/score2audio/
	cd build/linux && tar czf ../../dist/score2audio-linux-x86_64.tar.gz score2audio
	@ls -l dist

install: build tessdata
	install -Dm755 score2audio $(PREFIX)/bin/score2audio
	install -Dm644 tessdata/eng.traineddata $(PREFIX)/share/score2audio/tessdata/eng.traineddata
	install -Dm644 packaging/linux/score2audio.desktop $(PREFIX)/share/applications/score2audio.desktop
	@echo "Installed. Start it with: score2audio"

clean:
	rm -rf build dist score2audio score2audio.exe
