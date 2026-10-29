package main

import "time"

const (
	scanTimeout      = 15 * time.Second
	uninstallTimeout = 120 * time.Second

	// Search / results UX limits. The result limit applies independently per source.
	minSearchLen    = 2
	maxScanResults  = 200
	resultsPageSize = 12

	SourceAPT      = "APT"
	SourcePacman   = "Pacman"
	SourceFlatpak  = "Flatpak"
	SourceSnap     = "Snap"
	SourceAppImage = "AppImage"
	SourceWebApp   = "Web App"
)
