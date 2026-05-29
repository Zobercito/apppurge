package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Application states
const (
	stateInput = iota
	stateScanning
	stateResults
	stateModeSelect
	stateConfirm
	statePassword
	stateSudoAuth
	stateExecuting
	stateDone
	stateHelp
	statePackageInfo
	stateDryRun
)

// Uninstall modes
const (
	modeNormal   = "normal"
	modeComplete = "complete"
)

// Messages
type scanDoneMsg struct{ result ScanResult }
type uninstallDoneMsg struct{ err error }
type sudoAuthDoneMsg struct{ err error }
type pkgInfoMsg struct{ info PackageInfo }
type dryRunMsg struct {
	result DryRunResult
	err    error
}

// Model is the main Bubble Tea model.
type Model struct {
	state         int
	textInput     textinput.Model
	spinner       spinner.Model
	viewport      viewport.Model
	help          help.Model
	searchTerm    string
	results       []PackageResult
	allResults    []PackageResult
	cursor        int
	selected      PackageResult
	modeCursor    int
	mode          string
	err           error
	quitting      bool
	scanErrors    []ScanError
	filter        string
	width         int
	height        int
	pkgInfo       *PackageInfo
	dryResult     *DryRunResult
	passwordInput textinput.Model
}

// InitModel creates a new model, optionally with a pre-filled search term.
func InitModel(term string) Model {
	ti := textinput.New()
	ti.Placeholder = "e.g.: firefox, vlc, gimp..."
	ti.Width = 60
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	pi := textinput.New()
	pi.Placeholder = "sudo password"
	pi.EchoMode = textinput.EchoPassword
	pi.Width = 60
	pi.Focus()

	state := stateInput
	if term != "" {
		state = stateScanning
	}

	return Model{
		state:         state,
		textInput:     ti,
		spinner:       sp,
		searchTerm:    term,
		help:          help.New(),
		viewport:      viewport.New(80, 20),
		passwordInput: pi,
	}
}

func (m Model) Init() tea.Cmd {
	if m.state == stateScanning {
		return tea.Batch(m.spinner.Tick, doScan(m.searchTerm))
	}
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 5
		m.textInput.Width = 60
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			if m.state == stateHelp {
				m.state = stateInput
				return m, tea.ClearScreen
			} else {
				m.state = stateHelp
				return m, tea.ClearScreen
			}
		case "ctrl+c", "ctrl+z":
			m.quitting = true
			return m, tea.Quit
		}
	case scanDoneMsg:
		m.results = msg.result.Packages
		m.allResults = msg.result.Packages
		m.scanErrors = msg.result.Errors
		if len(m.results) == 0 {
			m.state = stateDone
			return m, nil
		}
		m.state = stateResults
		m.cursor = 0
		return m, nil
	case uninstallDoneMsg:
		m.err = msg.err
		m.state = stateDone
		return m, nil
	case sudoAuthDoneMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("incorrect password: %w", msg.err)
			m.state = statePassword
			m.passwordInput.SetValue("")
			m.passwordInput.Focus()
			return m, tea.ClearScreen
		}
		m.state = stateExecuting
		return m, tea.Batch(tea.ClearScreen, m.spinner.Tick, doUninstall(m.selected, m.mode))
	case pkgInfoMsg:
		m.pkgInfo = &msg.info
		return m, nil
	case dryRunMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateDone
			return m, nil
		}
		m.dryResult = &msg.result
		return m, nil
	}

	switch m.state {
	case stateInput:
		return m.updateInput(msg)
	case stateScanning, stateExecuting, stateSudoAuth:
		return m.updateSpinner(msg)
	case stateResults:
		return m.updateResults(msg)
	case stateModeSelect:
		return m.updateModeSelect(msg)
	case stateConfirm:
		return m.updateConfirm(msg)
	case statePassword:
		return m.updatePassword(msg)
	case stateHelp:
		return m.updateHelp(msg)
	case statePackageInfo:
		return m.updatePackageInfo(msg)
	case stateDryRun:
		return m.updateDryRun(msg)
	case stateDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func (m Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "?":
			m.state = stateHelp
			return m, nil
		case "enter":
			term := strings.TrimSpace(m.textInput.Value())
			if term == "" {
				return m, nil
			}
			m.searchTerm = term
			m.state = stateScanning
			return m, tea.Batch(m.spinner.Tick, doScan(term))
		case "esc":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateSpinner(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) updateResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.results) > 0 {
				m.selected = m.results[m.cursor]
				m.state = stateModeSelect
				m.modeCursor = 0
			}
		case "i":
			if len(m.results) > 0 {
				m.selected = m.results[m.cursor]
				m.pkgInfo = nil
				m.state = statePackageInfo
				return m, doGetPackageInfo(m.selected)
			}
		case "d":
			if len(m.results) > 0 {
				m.selected = m.results[m.cursor]
				m.dryResult = nil
				m.state = stateDryRun
				mode := m.mode
				if mode == "" {
					mode = modeNormal
				}
				return m, tea.Batch(m.spinner.Tick, doDryRun(m.selected, mode))
			}
		case "esc":
			m.state = stateInput
			m.textInput.Focus()
			m.searchTerm = ""
			m.textInput.SetValue("")
		case "1":
			m.filter = ""
			m.results = m.allResults
		case "2":
			m.filter = SourceAPT
			m.applyFilter()
		case "3":
			m.filter = SourceFlatpak
			m.applyFilter()
		case "4":
			m.filter = SourceSnap
			m.applyFilter()
		case "5":
			m.filter = SourceAppImage
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *Model) applyFilter() {
	if m.filter == "" {
		m.results = m.allResults
		return
	}
	var filtered []PackageResult
	for _, r := range m.allResults {
		if r.Source == m.filter {
			filtered = append(filtered, r)
		}
	}
	m.results = filtered
	m.cursor = 0
}

func (m Model) updateModeSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.modeCursor > 0 {
				m.modeCursor--
			}
		case "down", "j":
			if m.modeCursor < 2 {
				m.modeCursor++
			}
		case "enter":
			switch m.modeCursor {
			case 0:
				m.mode = modeNormal
				m.state = stateConfirm
			case 1:
				m.mode = modeComplete
				m.state = stateConfirm
			case 2:
				m.state = stateResults
			}
		case "esc":
			m.state = stateResults
		}
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch strings.ToLower(key.String()) {
		case "s", "y":
			if m.selected.Source == SourceAPT || m.selected.Source == SourceSnap {
				m.state = statePassword
				m.err = nil
				m.passwordInput.SetValue("")
				m.passwordInput.Focus()
				return m, tea.ClearScreen
			}
			m.state = stateExecuting
			return m, tea.Batch(tea.ClearScreen, m.spinner.Tick, doUninstall(m.selected, m.mode))
		case "n", "esc":
			m.state = stateModeSelect
		}
	}
	return m, nil
}

func (m Model) updatePassword(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			password := m.passwordInput.Value()
			if password == "" {
				return m, nil
			}
			m.passwordInput.SetValue("")
			m.state = stateSudoAuth
			return m, tea.Batch(tea.ClearScreen, m.spinner.Tick, doSudoPassword(password))
		case "esc":
			m.state = stateModeSelect
			m.passwordInput.SetValue("")
			m.err = nil
		}
	}
	var cmd tea.Cmd
	m.passwordInput, cmd = m.passwordInput.Update(msg)
	return m, cmd
}

func (m Model) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			m.state = stateInput
			return m, tea.ClearScreen
		}
	}
	return m, nil
}

func (m Model) updatePackageInfo(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q", "enter", "i":
			m.state = stateResults
		}
	}
	return m, nil
}

func (m Model) updateDryRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q", "enter", "d":
			m.state = stateResults
		}
	}
	return m, nil
}

func (m Model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "esc" {
			m.state = stateInput
			m.textInput.Focus()
			m.textInput.SetValue("")
			m.err = nil
			m.results = nil
			m.allResults = nil
			return m, nil
		}
		m.state = stateInput
		m.textInput.Focus()
		m.textInput.SetValue("")
		m.err = nil
		m.results = nil
		m.allResults = nil
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return dimStyle.Render("Operation cancelled.") + "\n"
	}

	separator := "\n" + dimStyle.Render("────────────────────────────────────────────────")

	if m.state == stateHelp {
		return m.viewHelp() + separator
	}

	switch m.state {
	case stateInput:
		return m.viewInput() + separator
	case stateScanning:
		return m.viewScanning() + separator
	case stateResults:
		return m.viewResults() + separator
	case stateModeSelect:
		return m.viewModeSelect() + separator
	case stateConfirm:
		return m.viewConfirm() + separator
	case statePassword:
		return m.viewPassword() + separator
	case stateSudoAuth:
		return m.viewSudoAuth() + separator
	case stateExecuting:
		return m.viewExecuting() + separator
	case stateDone:
		return m.viewDone() + separator
	case statePackageInfo:
		return m.viewPackageInfo() + separator
	case stateDryRun:
		return m.viewDryRun() + separator
	}
	return ""
}

func (m Model) viewInput() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Interactive Uninstaller"))
	b.WriteString("\n\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("esc: quit • help: ?"))
	return b.String()
}

func (m Model) viewScanning() string {
	return fmt.Sprintf("%s\n\n%s Searching system for \"%s\"...\n",
		renderBanner(),
		m.spinner.View(),
		m.searchTerm,
	)
}

func (m Model) viewResults() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%d results for \"%s\"", len(m.results), m.searchTerm)))
	b.WriteString(" ")
	if m.filter == "" {
		b.WriteString(sourceTag("ALL").Render("[Filter: ALL]"))
	} else {
		b.WriteString(sourceTag(m.filter).Render("[Filter: " + m.filter + "]"))
	}
	b.WriteString("\n\n")

	for i, r := range m.results {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("▸ ")
		}
		b.WriteString(cursor + formatResult(r, i == m.cursor) + "\n")
	}

	b.WriteString("\n")
	if len(m.scanErrors) > 0 {
		for _, e := range m.scanErrors {
			b.WriteString(warningStyle.Render("⚠ "+e.Error()) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("esc: back • 1-5: filter • help: ?"))
	return b.String()
}

func (m Model) viewModeSelect() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Selected package: %s\n\n", selectedStyle.Render(m.selected.Name)))
	b.WriteString(subtitleStyle.Render("How would you like to proceed?"))
	b.WriteString("\n\n")

	options := []string{
		"Normal Uninstall (keep your settings)",
		"Complete Uninstall (delete everything)",
		"Back to results",
	}
	for i, opt := range options {
		cursor := "  "
		style := dimStyle
		if i == m.modeCursor {
			cursor = selectedStyle.Render("▸ ")
			style = selectedStyle
		}
		b.WriteString(cursor + style.Render(opt) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑/↓: navigate • enter: select • esc: back"))
	return b.String()
}

func (m Model) viewConfirm() string {
	modeText := "Normal Uninstall"
	if m.mode == modeComplete {
		modeText = "Complete Uninstall"
	}
	alert := fmt.Sprintf("Uninstall \"%s\" [%s]\nMode: %s\n\nAre you sure?",
		m.selected.Name, m.selected.Source, modeText)
	return fmt.Sprintf("%s\n\n%s\n\n%s\n",
		renderBanner(),
		warningStyle.Render(alert),
		dimStyle.Render("y/s: confirm • n/esc: cancel"),
	)
}

func (m Model) viewPassword() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Package: %s\n\n", selectedStyle.Render(m.selected.Name)))
	b.WriteString(subtitleStyle.Render("Sudo password required to uninstall:"))
	b.WriteString("\n\n")
	b.WriteString(m.passwordInput.View())
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(errorStyle.Render("✗ "+m.err.Error()) + "\n\n")
	}
	b.WriteString(dimStyle.Render("enter: confirm • esc: cancel"))
	return b.String()
}

func (m Model) viewSudoAuth() string {
	return fmt.Sprintf("%s\n\n%s Verifying password...\n",
		renderBanner(),
		m.spinner.View(),
	)
}

func (m Model) viewExecuting() string {
	return fmt.Sprintf("%s\n\n%s Uninstalling %s...\n",
		renderBanner(),
		m.spinner.View(),
		m.selected.Name,
	)
}

func (m Model) viewDone() string {
	if len(m.results) == 0 && m.err == nil {
		return fmt.Sprintf("%s\n\n%s\n\n%s\n",
			renderBanner(),
			dimStyle.Render("Nothing found matching \""+m.searchTerm+"\"."),
			dimStyle.Render("esc: back"),
		)
	}
	if m.err != nil {
		return fmt.Sprintf("%s\n\n%s\n%s\n\n%s\n",
			renderBanner(),
			errorStyle.Render("✗ Error during uninstall:"),
			errorStyle.Render(m.err.Error()),
			dimStyle.Render("Press any key to continue."),
		)
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s\n",
		renderBanner(),
		successStyle.Render("✓ Done! \""+m.selected.Name+"\" has been successfully removed."),
		dimStyle.Render("Press any key to continue."),
	)
}

func (m Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Help"))
	b.WriteString("\n\n")
	b.WriteString(subtitleStyle.Render("Navigation:"))
	b.WriteString("\n")
	b.WriteString("  ↑/k  Move up in list\n")
	b.WriteString("  ↓/j  Move down in list\n")
	b.WriteString("  enter  Select/Confirm\n")
	b.WriteString("  esc  Go back / Quit\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Actions (in results):"))
	b.WriteString("\n")
	b.WriteString("  i  View package info\n")
	b.WriteString("  d  Dry-run (simulate uninstall)\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Filters (in results):"))
	b.WriteString("\n")
	b.WriteString("  1  Show all (no filter)\n")
	b.WriteString("  2  Show only APT\n")
	b.WriteString("  3  Show only Flatpak\n")
	b.WriteString("  4  Show only Snap\n")
	b.WriteString("  5  Show only AppImage\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Uninstall modes:"))
	b.WriteString("\n")
	b.WriteString("  Normal:   Remove the program, keep config files\n")
	b.WriteString("  Complete: Remove the program and ALL its data\n")
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press esc or ? to go back"))
	return b.String()
}

func (m Model) viewPackageInfo() string {
	if m.pkgInfo == nil {
		return fmt.Sprintf("%s\n\n%s Loading info...\n",
			renderBanner(),
			m.spinner.View(),
		)
	}
	info := m.pkgInfo
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Package Info"))
	b.WriteString("\n\n")
	b.WriteString("  Name:        " + selectedStyle.Render(info.Name) + "\n")
	if info.Version != "" {
		b.WriteString("  Version:     " + info.Version + "\n")
	}
	if info.InstalledSize != "" {
		b.WriteString("  Size:        " + info.InstalledSize + "\n")
	}
	if info.Description != "" {
		maxWidth := m.width - 16 // "  Description: " = 15 + 1 space
		if maxWidth < 30 {
			maxWidth = 60
		}
		wrapped := wordWrap(info.Description, maxWidth)
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			if i == 0 {
				b.WriteString("  Description: " + line + "\n")
			} else {
				b.WriteString("               " + line + "\n")
			}
		}
	}
	if len(info.Dependents) > 0 {
		b.WriteString("\n")
		b.WriteString(highlightStyle.Render("  ⚠ Reverse dependencies ("+fmt.Sprintf("%d", len(info.Dependents))+"):") + "\n")
		for _, dep := range info.Dependents {
			b.WriteString("    - " + dep + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press esc, enter or i to go back"))
	return b.String()
}

func (m Model) viewDryRun() string {
	if m.dryResult == nil {
		return fmt.Sprintf("%s\n\n%s Simulating uninstall...\n",
			renderBanner(),
			m.spinner.View(),
		)
	}
	result := m.dryResult

	var b strings.Builder
	b.WriteString(renderBannerSuffix("Dry Run (Simulation)"))
	b.WriteString("\n\n")
	b.WriteString("  Package: " + selectedStyle.Render(m.selected.Name) + " [" + m.selected.Source + "]\n")
	modeText := "Normal"
	if m.mode == modeComplete {
		modeText = "Complete (purge)"
	}
	b.WriteString("  Mode:    " + modeText + "\n")
	b.WriteString("  Estimated space to free: " + successStyle.Render(FormatBytes(result.EstimateBytes)) + "\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Commands that would be executed:"))
	b.WriteString("\n")
	for _, cmd := range result.Commands {
		b.WriteString("  $ " + cmd + "\n")
	}
	if len(result.DirsToRemove) > 0 {
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Directories that would be removed:"))
		b.WriteString("\n")
		for _, dir := range result.DirsToRemove {
			b.WriteString("  - " + dir + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press esc, enter or d to go back"))
	return b.String()
}

func sourceTag(source string) lipgloss.Style {
	switch source {
	case SourceAPT:
		return sourceAPT
	case SourceFlatpak:
		return sourceFlatpak
	case SourceSnap:
		return sourceSnap
	case SourceAppImage:
		return sourceAppImage
	case "ALL":
		return sourceAll
	}
	return dimStyle
}

// Helper to format a result line with source-colored tag.
func formatResult(r PackageResult, active bool) string {
	var tag string
	switch r.Source {
	case SourceAPT:
		tag = sourceAPT.Render("[APT]")
	case SourceFlatpak:
		tag = sourceFlatpak.Render("[Flatpak]")
	case SourceSnap:
		tag = sourceSnap.Render("[Snap]")
	case SourceAppImage:
		tag = sourceAppImage.Render("[AppImage]")
	}

	name := r.Name
	if r.Source == SourceFlatpak && r.ID != "" {
		name = fmt.Sprintf("%s (%s)", r.Name, r.ID)
	}
	if r.Source == SourceAppImage && r.Path != "" {
		name = fmt.Sprintf("%s → %s", r.Name, r.Path)
	}

	if active {
		return tag + " " + selectedStyle.Render(name)
	}
	return tag + " " + name
}

// wordWrap breaks a string into multiple lines at word boundaries, each
// line at most width characters wide.
func wordWrap(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var wrapped strings.Builder
	words := strings.Fields(s)
	lineLen := 0
	for _, word := range words {
		space := 1
		if lineLen == 0 {
			space = 0
		}
		if lineLen+space+len(word) > width {
			if lineLen > 0 {
				wrapped.WriteByte('\n')
			}
			wrapped.WriteString(word)
			lineLen = len(word)
		} else {
			if lineLen > 0 {
				wrapped.WriteByte(' ')
			}
			wrapped.WriteString(word)
			lineLen += space + len(word)
		}
	}
	return wrapped.String()
}

// Commands
func doScan(term string) tea.Cmd {
	return func() tea.Msg {
		result := ScanAll(term)
		return scanDoneMsg{result: result}
	}
}

func doSudoPassword(password string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("sudo", "-S", "-v")
		cmd.Stdin = strings.NewReader(password + "\n")
		if err := cmd.Run(); err != nil {
			return sudoAuthDoneMsg{err: err}
		}
		return sudoAuthDoneMsg{err: nil}
	}
}

func doUninstall(pkg PackageResult, mode string) tea.Cmd {
	return func() tea.Msg {
		err := Uninstall(pkg, mode)
		return uninstallDoneMsg{err: err}
	}
}

func doGetPackageInfo(pkg PackageResult) tea.Cmd {
	return func() tea.Msg {
		info := GetPackageInfo(pkg)
		return pkgInfoMsg{info: info}
	}
}

func doDryRun(pkg PackageResult, mode string) tea.Cmd {
	return func() tea.Msg {
		result, err := DryRun(pkg, mode)
		return dryRunMsg{result: result, err: err}
	}
}
