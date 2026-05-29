# 🗑 AppPurge

**Interactive TUI uninstaller for Linux** — busca y desinstala programas desde APT, Flatpak, Snap y AppImage con una interfaz bonita en terminal.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Screenshot

```
                ◢◤ AppPurge ◢◤

  ┌────────────────────────────────────────────┐
  │  Ej: firefox, vlc, gimp...                 │
  └────────────────────────────────────────────┘

  enter: buscar • esc: salir • ?: ayuda
```

## Installation

```bash
git clone https://github.com/Zobercito/apppurge.git
cd apppurge
make build          # builds to build/appurge
sudo make install   # copies to /usr/local/bin/
```

Or just:

```bash
go install github.com/Zobercito/apppurge@latest
```

## Usage

```bash
appurge              # interactive mode (prompts for package name)
appurge firefox      # search directly for a package
appurge -h           # show help
```

## Supported Sources

| Source | Description |
|--------|-------------|
| **APT** | System packages (Debian/Ubuntu) |
| **Flatpak** | Flatpak applications |
| **Snap** | Snap packages |
| **AppImage** | AppImage files in `~/Applications`, `~/Downloads`, `~/.local/share/Applications` |

## Uninstall Modes

- **Normal** — Removes the program, keeps your configuration files
- **Complete** — Removes the program and all its data (config, cache, local state)

## Features

- Interactive TUI with keyboard navigation (`↑/↓`, `enter`, `esc`)
- Real-time search across all package managers in parallel
- Per-source filtering (`1`=APT, `2`=Flatpak, `3`=Snap, `4`=AppImage)
- Dry-run mode to preview what would be deleted
- Package info viewer (version, size, description, reverse dependencies)
- Uninstall history stored in `~/.config/appurge/history.json`
- Safety checks: protected packages list, name validation, path validation
- Color-coded sources: APT=teal, Flatpak=violet, Snap=orange, AppImage=pink

## Development

```bash
make build    # compile
make test     # run tests
make lint     # golangci-lint
make fmt      # go fmt
make clean    # remove build artifacts
```

## Dependencies

- Go 1.18+
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) v0.25.0
- [Bubbles](https://github.com/charmbracelet/bubbles) v0.18.0
- [Lipgloss](https://github.com/charmbracelet/lipgloss) v0.10.0

## License

GNU General Public License v3.0 — see [LICENSE](LICENSE).
