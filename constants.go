package main

import "time"

const (
	scanTimeout      = 15 * time.Second
	uninstallTimeout = 120 * time.Second

	SourceAPT      = "APT"
	SourceFlatpak  = "Flatpak"
	SourceSnap     = "Snap"
	SourceAppImage = "AppImage"
)
