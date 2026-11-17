.PHONY: build run install clean

# Where `make install` copies the app.
APP_DIR ?= /Applications
APP := build/HeyGUIGo.app

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

# Removes build and the whole Go build cache, including other projects'.
clean:
	rm -rf build
	go clean -cache
