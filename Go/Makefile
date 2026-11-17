.PHONY: build run install linux clean

# Where `make install` copies the app.
APP_DIR ?= /Applications
APP := build/HeyGUIGo.app

APP_ID := com.ssig33.HeyGUIGo

# Where `make linux` installs the binary, icons and desktop entry.
BIN_DIR ?= $(HOME)/.local/bin
DATA_DIR ?= $(HOME)/.local/share
LINUX_BIN := build/linux/hey-gui-go
ICON_SIZES := 16 32 128 256 512

build:
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	go build -trimpath -ldflags "-s -w" -o $(APP)/Contents/MacOS/HeyGUIGo .
	cp Resources/Info.plist $(APP)/Contents/Info.plist
	iconutil -c icns -o $(APP)/Contents/Resources/AppIcon.icns Resources/AppIcon.iconset

run: build
	open $(APP)

install: build
	rm -rf "$(APP_DIR)/HeyGUIGo.app"
	cp -R $(APP) "$(APP_DIR)/HeyGUIGo.app"

linux:
	mkdir -p build/linux
	go build -trimpath -ldflags "-s -w -X gioui.org/app.ID=$(APP_ID)" -o $(LINUX_BIN) .
	install -Dm755 $(LINUX_BIN) "$(BIN_DIR)/hey-gui-go"
	for s in $(ICON_SIZES); do \
		install -Dm644 Resources/AppIcon.iconset/icon_$${s}x$${s}.png "$(DATA_DIR)/icons/hicolor/$${s}x$${s}/apps/$(APP_ID).png"; \
	done
	mkdir -p "$(DATA_DIR)/applications"
	sed "s|@BIN@|$(BIN_DIR)/hey-gui-go|" Resources/$(APP_ID).desktop > "$(DATA_DIR)/applications/$(APP_ID).desktop"
	-update-desktop-database "$(DATA_DIR)/applications"
	-gtk-update-icon-cache -q -t "$(DATA_DIR)/icons/hicolor"

# Removes build and the whole Go build cache, including other projects'.
clean:
	rm -rf build
	go clean -cache
