# mdify — derleme ve release
#
#   make build                    # yerel derleme (./mdify veya mdify.exe)
#   make test
#   make dist    VERSION=0.2.0    # 6 platform: dist/v0.2.0/*.zip|*.tar.gz + checksums.txt
#   make release VERSION=0.2.0    # test + dist + tag + push + GitHub release
#   make clean
#
# Gerekenler: go, git, gh (gh auth login), tar ve POSIX sh.
# Windows: Git for Windows'un sh/tar/sha256sum'ı PATH'te olmalı; make için
#   winget install ezwinports.make   (veya choco install make)

REPO     ?= NetYazilim/mdify
VERSION  ?=
TAG      := v$(patsubst v%,%,$(VERSION))
VER      := $(patsubst v%,%,$(VERSION))
DIST     := dist/$(TAG)
LDFLAGS  := -s -w -X main.version=$(if $(VER),$(VER),dev)
PLATFORMS := windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

ifeq ($(OS),Windows_NT)
  EXE := .exe
  # Windows'ta zip: PowerShell'in Compress-Archive'i
  ZIP = powershell -NoProfile -Command "Compress-Archive -Force -Path '$(1)/*' -DestinationPath '$(2)'"
else
  EXE :=
  ZIP = (cd $(1) && zip -q -r $(CURDIR)/$(2) .)
endif

# sha256sum (Linux, Git Bash) veya shasum (macOS)
SHA256 := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")

.PHONY: all build test dist checksums release clean check-version check-clean

all: build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o mdify$(EXE) .

test:
	go test ./...

check-version:
ifeq ($(strip $(VERSION)),)
	$(error VERSION gerekli, ör. make $(MAKECMDGOALS) VERSION=0.2.0)
endif

dist: check-version test
	@mkdir -p $(DIST)
	@rm -rf $(DIST)/.build $(DIST)/*.zip $(DIST)/*.tar.gz $(DIST)/checksums.txt
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		name=mdify_$(VER)_$${os}_$${arch}; \
		work=$(DIST)/.build/$$name; \
		bin=mdify; [ $$os = windows ] && bin=mdify.exe; \
		echo "==> $$os/$$arch"; \
		mkdir -p $$work; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$work/$$bin .; \
		cp README.md $$work/; \
		if [ $$os = windows ]; then \
			$(call ZIP,$$work,$(DIST)/$$name.zip); \
		else \
			tar -czf $(DIST)/$$name.tar.gz -C $$work $$bin README.md; \
		fi; \
	done
	@rm -rf $(DIST)/.build
	@$(MAKE) --no-print-directory checksums VERSION=$(VER)
	@echo "==> arşivler: $(DIST)"

checksums: check-version
	@cd $(DIST) && $(SHA256) *.zip *.tar.gz > checksums.txt

check-clean:
	@test -z "$$(git status --porcelain --untracked-files=no)" || { echo "Commit edilmemiş değişiklikler var; önce commit edin."; exit 1; }
	@test -z "$$(git tag --list $(TAG))" || { echo "$(TAG) tag'i zaten var."; exit 1; }
	@gh auth status >/dev/null

release: check-version check-clean dist
	git tag -a $(TAG) -m "mdify $(TAG)"
	git push origin HEAD
	git push origin $(TAG)
	gh release create $(TAG) --repo $(REPO) --title "mdify $(TAG)" --verify-tag \
		$(if $(wildcard $(DIST)/RELEASE_NOTES.md),--notes-file $(DIST)/RELEASE_NOTES.md,--generate-notes) \
		$(DIST)/*.zip $(DIST)/*.tar.gz $(DIST)/checksums.txt
	@echo "==> yayınlandı: https://github.com/$(REPO)/releases/tag/$(TAG)"

clean:
	rm -f mdify mdify.exe
	rm -rf dist/*/.build
