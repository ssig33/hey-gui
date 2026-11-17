# hey-gui

A personal GUI for reading the HEY Imbox. It runs the [hey CLI](https://github.com/basecamp/hey-cli)
as a child process. You can read threads, search, and mark threads as seen. There are two
implementations of the same features.

| Directory | Implementation | OS |
| --- | --- | --- |
| `Go/` | Go + [Gio](https://gioui.org) | macOS, Linux |
| `Apple/` | SwiftUI | macOS 27 or later |

## Requirements

- The hey CLI is installed and logged in with `hey auth login`.
- GUI apps don't inherit the shell's `PATH`, so `hey` is looked up with `command -v hey` in a
  login shell and in common install locations. If it isn't found, set its location in Settings.

## Build and install

### Go/

```sh
cd Go
make build     # builds build/HeyGUIGo.app (macOS)
make install   # copies it to /Applications (override with APP_DIR)
make linux     # Linux: installs ~/.local/bin/hey-gui-go, icons and a desktop entry under ~/.local/share
```

`make linux` installs to `BIN_DIR` and `DATA_DIR`, which can be overridden. Building on Linux
needs the development headers Gio uses: Vulkan, Wayland, X11, xkbcommon and so on. On Arch
Linux, these are `vulkan-headers`, `wayland`, `libx11`, `libxkbcommon-x11`, `libxcursor`,
`libxfixes` and `mesa`.

Tests:

```sh
cd Go
go vet ./... && go test ./...
```

### Apple/

Requires xcodegen and Xcode.

```sh
cd Apple
make build     # generates the project with xcodegen and builds Release
make install   # copies it to /Applications (override with APP_DIR)
```

## Cache and settings

The hey CLI output for the Imbox and threads is cached so they show instantly on the next launch.

- Go/: the cache is in `os.UserCacheDir()/hey-gui` (`~/Library/Caches/hey-gui` on macOS,
  `~/.cache/hey-gui` on Linux). Settings are in `os.UserConfigDir()/hey-gui/config.json`.
- Apple/: the cache is in `~/Library/Caches/com.ssig33.HeyGUI`.
