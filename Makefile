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

-include .env

INSTALL_DIR ?= $(HOME_DIR)/.segmentstream/bin
BINARY := segmentstream$(EXE_EXT)
LDFLAGS :=
ifneq ($(SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_ID),)
LDFLAGS += -X github.com/segmentstream/segmentstream-cli/internal/warehouse/bigquery/googleoauth.desktopClientID=$(SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_ID)
endif
ifneq ($(SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_SECRET),)
LDFLAGS += -X github.com/segmentstream/segmentstream-cli/internal/warehouse/bigquery/googleoauth.desktopClientSecret=$(SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_SECRET)
endif

.PHONY: install
install:
	$(MKDIR_P)
	@"$(GO)" build -ldflags "$(LDFLAGS)" -o "$(INSTALL_DIR)/$(BINARY)" ./cmd/segmentstream
	@echo segmentstream installed to "$(INSTALL_DIR)/$(BINARY)"
