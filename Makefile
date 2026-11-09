# Build score2audio. Needs Go 1.24 or newer (on Arch Linux: sudo pacman -S go); an
# older Go fetches the newer Go named in go.mod by itself, for its security fixes.
#
#   make            build for this computer
#   make test       run the tests
#   make run        build and start it
#   make dist       build download packages for Windows, Mac and Linux
#   make install    install for this user on Linux (into ~/.local)

VERSION ?= $(or $(shell git describe --tags --always 2>/dev/null | sed "s/^v//"),dev)
LDFLAGS := -s -w -X main.appVersion=$(VERSION)
GO      ?= go
# English language data for Tesseract, pinned to a known commit and checked
# against its SHA-256, so a changed or tampered download is never shipped.
TESSDATA_COMMIT := ced78752cc61322fb554c280d13360b35b8684e4
TESSDATA_SHA256 := daa0c97d651c19fba3b25e81317cd697e9908c8208090c94c3905381c23fc047
TESSDATA_URL := https://github.com/tesseract-ocr/tessdata/raw/$(TESSDATA_COMMIT)/eng.traineddata
# Verovio, the music engraver used for videos: the unmodified npm build,
# pinned to a version and checked against the SHA-512 npm publishes for it.
# It ships as its own file (LGPL), next to the program.
VEROVIO_VERSION := 6.4.1
VEROVIO_SHA512 := fb97b1b80eb7002d051c5a342ffe0195c17a12910e588a7cd69c33fda7ccba9c6cb3fb6cadd54bc57f0faa1f5017350e5dc07898ff41a31e3173e86b90a37e2c
VEROVIO_URL := https://registry.npmjs.org/verovio/-/verovio-$(VEROVIO_VERSION).tgz
PREFIX  ?= $(HOME)/.local

.PHONY: all build test vulncheck run dist clean install tessdata verovio

all: build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o score2audio$(if $(filter windows,$(shell $(GO) env GOOS)),.exe,) .

test:
	$(GO) vet ./...
	$(GO) test ./...

# Check the code and the Go standard library it uses for known vulnerabilities.
vulncheck:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

run: build tessdata verovio
	./score2audio

tessdata/eng.traineddata:
	mkdir -p tessdata
	curl -fsSL -o $@.download $(TESSDATA_URL)
	@if command -v sha256sum >/dev/null; then sum=$$(sha256sum $@.download); else sum=$$(shasum -a 256 $@.download); fi; \
	  case "$$sum" in $(TESSDATA_SHA256)*) ;; *) echo "eng.traineddata does not match its expected SHA-256; not using it."; rm -f $@.download; exit 1;; esac
	mv $@.download $@

tessdata: tessdata/eng.traineddata

verovio/verovio-toolkit-wasm.js:
	mkdir -p verovio
	curl -fsSL -o verovio/package.tgz.download $(VEROVIO_URL)
	@if command -v sha512sum >/dev/null; then sum=$$(sha512sum verovio/package.tgz.download); else sum=$$(shasum -a 512 verovio/package.tgz.download); fi; \
	  case "$$sum" in $(VEROVIO_SHA512)*) ;; *) echo "The Verovio package does not match its expected SHA-512; not using it."; rm -f verovio/package.tgz.download; exit 1;; esac
	tar -xzf verovio/package.tgz.download -C verovio --strip-components=2 package/dist/verovio-toolkit-wasm.js
	rm verovio/package.tgz.download
	cp third_party/verovio/COPYING third_party/verovio/COPYING.LESSER third_party/verovio/README.md verovio/

verovio: verovio/verovio-toolkit-wasm.js

dist: test tessdata verovio
	rm -rf build dist && mkdir -p dist
	# Windows
	mkdir -p build/windows/tessdata
	GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/windows/score2audio.exe .
	cp tessdata/eng.traineddata build/windows/tessdata/
	cp -r verovio build/windows/verovio
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
	cp -r verovio build/mac/score2audio/verovio
	cp README.md LICENSE build/mac/score2audio/
	cd build/mac && zip -qry ../../dist/score2audio-mac.zip score2audio
	# Linux
	mkdir -p build/linux/score2audio/tessdata
	GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/linux/score2audio/score2audio .
	cp tessdata/eng.traineddata build/linux/score2audio/tessdata/
	cp -r verovio build/linux/score2audio/verovio
	cp README.md LICENSE packaging/linux/score2audio.desktop build/linux/score2audio/
	cd build/linux && tar czf ../../dist/score2audio-linux-x86_64.tar.gz score2audio
	@ls -l dist

install: build tessdata verovio
	install -Dm755 score2audio $(PREFIX)/bin/score2audio
	install -Dm644 tessdata/eng.traineddata $(PREFIX)/share/score2audio/tessdata/eng.traineddata
	for f in verovio/*; do install -Dm644 "$$f" "$(PREFIX)/share/score2audio/$$f"; done
	install -Dm644 packaging/linux/score2audio.desktop $(PREFIX)/share/applications/score2audio.desktop
	@echo "Installed. Start it with: score2audio"

clean:
	rm -rf build dist score2audio score2audio.exe
