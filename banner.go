package main

import "github.com/charmbracelet/lipgloss"

var bannerTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FFFFFF"))

var (
	dashLine        = dimStyle.Render("────────────────────────────────────────────────")
	bannerTitleLine = bannerTitleStyle.Render("                ◢◤ AppPurge ◢◤")
)

func renderBanner() string {
	return dashLine + "\n" + bannerTitleLine + "\n" + dashLine
}

func renderBannerSuffix(suffix string) string {
	return renderBanner() + "\n" + subtitleStyle.Render(suffix)
}
