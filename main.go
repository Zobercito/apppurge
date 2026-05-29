package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const helpText = `🗑  AppPurge - Interactive program uninstaller

Usage:
  appurge              Start interactive mode (prompts for package name)
  appurge <name>       Search directly for the given program
  appurge -h, --help   Show this help

Supported sources:
  • APT (system packages)
  • Flatpak
  • Snap
  • AppImage (.AppImage files in your home)

Uninstall modes:
  • Normal:   Remove the program, keep your config files
  • Complete: Remove the program and its data (conservative purge)
`

func main() {
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "-h" || arg == "--help" {
			fmt.Print(helpText)
			os.Exit(0)
		}
	}

	term := ""
	if len(os.Args) > 1 {
		term = strings.Join(os.Args[1:], " ")
	}

	p := tea.NewProgram(InitModel(term), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
