ifeq ($(OS),Windows_NT)
EXE_EXT := .exe
HOME_DIR := $(subst \,/,$(USERPROFILE))
GO ?= C:/Program Files/Go/bin/go.exe
MKDIR_P = powershell -NoProfile -ExecutionPolicy Bypass -Command "New-Item -ItemType Directory -Force -Path '$(INSTALL_DIR)' | Out-Null"
else
EXE_EXT :=
HOME_DIR := $(HOME)
GO ?= go
MKDIR_P = mkdir -p "$(INSTALL_DIR)"
endif

INSTALL_DIR ?= $(HOME_DIR)/.segmentstream/bin
BINARY := segmentstream$(EXE_EXT)

.PHONY: install
install:
	$(MKDIR_P)
	"$(GO)" build -o "$(INSTALL_DIR)/$(BINARY)" ./cmd/segmentstream
	@echo segmentstream installed to "$(INSTALL_DIR)/$(BINARY)"
