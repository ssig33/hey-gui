.PHONY: generate build run install icon clean

# Where `make install` copies the Release build.
APP_DIR ?= /Applications
APP := build/Build/Products/Release/HeyGUI.app

generate:
	xcodegen generate

build: generate
	xcodebuild -scheme HeyGUI -destination 'platform=macOS' -configuration Release -derivedDataPath build -allowProvisioningUpdates build

run: build
	open $(APP)

install: build
	rm -rf "$(APP_DIR)/HeyGUI.app"
	cp -R $(APP) "$(APP_DIR)/HeyGUI.app"

# Regenerates the app icon PNGs.
icon:
	swift Scripts/make-icon.swift Resources/Assets.xcassets/AppIcon.appiconset

# Removes build and all of Xcode's DerivedData, including other projects'.
clean:
	rm -rf build
	rm -rf ~/Library/Developer/Xcode/DerivedData/*
