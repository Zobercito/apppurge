# Changelog

All notable changes to AppPurge are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-10-29

### Added

- Pacman and Omarchy Web App sources, in addition to APT, Flatpak, Snap, and
  AppImage.
- Non-interactive CLI flags: `--source`, `--dry-run`, `--complete`, and
  `--version`.
- Package details view with version, size, and reverse dependencies.
- Uninstall history of the last 100 attempts, persisted to
  `~/.config/appurge/history.json`.
- Second confirmation step for high-impact removals: when dependents or extra
  packages are affected, the exact program name must be typed.
- Scrollable impact review with `PgUp`/`PgDn`, covering dependents, extra
  packages, and data or menu entries that will be removed.
- Complete mode for AppImages also removes matching user `.desktop` launchers
  whose `Exec` field points directly to the file.
- Sudo password prompt in the TUI.
- Unit tests for CLI parsing, UI flows, desktop entries, scanning, dry-run, and
  uninstaller helpers.

### Changed

- UI and README translated to English; navigation and result highlighting
  improved.
- Package search matches aliases and supports per-source filters.
- The confirmation preview no longer counts the selected package as an extra
  package.
- APT reverse-dependency parsing ignores non-dependency output lines.
- Build and CI updated to Go 1.27 and golangci-lint v2.

### Fixed

- High-impact removals cannot proceed when dependencies or the required preview
  cannot be verified.
- Uninstall impact lists are navigable across multiple pages and while resizing
  the terminal.

### Security

- Destructive actions stay blocked when preflight data is incomplete.
- Package names are validated before being passed to system package managers.
- Protected system packages cannot be removed.

## [1.0.0] - 2026-05-28

### Added

- Initial release: interactive TUI uninstaller for APT, Flatpak, Snap, and
  AppImage.
- Normal and complete uninstall modes.
- Local uninstall history.
