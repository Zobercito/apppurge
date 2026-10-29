package main

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	stateFinalConfirm
	statePassword
	stateSudoAuth
	stateExecuting
	stateDone
	stateHelp
	statePackageInfo
	stateDryRun
	stateHistory
)

// Uninstall modes
const (
	modeNormal   = "normal"
	modeComplete = "complete"
)

// Messages
type scanDoneMsg struct{ result ScanResult }
type catalogDoneMsg struct{ result ScanResult }
type uninstallDoneMsg struct{ err error }
type sudoAuthDoneMsg struct{ err error }
type pkgInfoMsg struct{ info PackageInfo }
type dryRunMsg struct {
	result DryRunResult
	err    error
}
type confirmDepsMsg struct {
	deps         []string
	depsKnown    bool
	depsErr      string
	removals     []string
	removalErr   string
	dataDirs     []string
	desktopFiles []string
}
type historyMsg struct {
	history History
	err     error
}

// Model is the main Bubble Tea model.
type Model struct {
	state               int
	textInput           textinput.Model
	spinner             spinner.Model
	searchTerm          string
	results             []PackageResult
	allResults          []PackageResult
	cursor              int
	selected            PackageResult
	modeCursor          int
	mode                string
	err                 error
	quitting            bool
	scanErrors          []ScanError
	filter              string
	sourceFilter        string
	width               int
	height              int
	pkgInfo             *PackageInfo
	dryResult           *DryRunResult
	passwordInput       textinput.Model
	truncated           bool
	helpReturnState     int
	confirmDeps         []string
	confirmDepsKnown    bool
	confirmDepsErr      string
	confirmRemovals     []string
	confirmRemovalErr   string
	confirmDataDirs     []string
	confirmDesktopFiles []string
	confirmDepsReady    bool
	finalConfirmInput   textinput.Model
	finalConfirmErr     string
	finalConfirmPage    int
	history             History
	historyErr          error
	historyCursor       int
	catalog             []PackageResult
	catalogErrors       []ScanError
	catalogLoaded       bool
	catalogLoading      bool
	suggestions         []PackageResult
	suggestionCursor    int
}

// InitModel creates a new model, optionally with a pre-filled search term.
func InitModel(term string) Model {
	ti := textinput.New()
	ti.Placeholder = "p. ej.: firefox, vlc, gimp..."
	ti.Width = 60
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	pi := textinput.New()
	pi.Placeholder = "Contraseña de sudo"
	pi.EchoMode = textinput.EchoPassword
	pi.Width = 60
	pi.Focus()

	finalConfirmInput := textinput.New()
	finalConfirmInput.Placeholder = "Escribe el nombre exacto del programa"
	finalConfirmInput.Width = 60

	state := stateInput
	if term != "" {
		state = stateScanning
	}

	return Model{
		state:             state,
		textInput:         ti,
		spinner:           sp,
		searchTerm:        term,
		passwordInput:     pi,
		finalConfirmInput: finalConfirmInput,
		catalogLoading:    true,
	}
}

func (m Model) Init() tea.Cmd {
	if m.state == stateScanning {
		return tea.Batch(m.spinner.Tick, doScanInstalled())
	}
	return tea.Batch(textinput.Blink, doScanInstalled())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = 60
		if m.state == stateFinalConfirm {
			lastPage := (len(m.finalConfirmItems()) - 1) / m.finalConfirmPageSize()
			if lastPage < 0 {
				lastPage = 0
			}
			if m.finalConfirmPage > lastPage {
				m.finalConfirmPage = lastPage
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			if m.state == stateHelp {
				m.state = m.helpReturnState
				return m, tea.ClearScreen
			}
			if m.state != statePassword && m.state != stateSudoAuth &&
				m.state != stateExecuting && m.state != stateScanning {
				m.helpReturnState = m.state
				m.state = stateHelp
				return m, tea.ClearScreen
			}
		case "ctrl+c", "ctrl+z":
			m.quitting = true
			return m, tea.Quit
		}
	case scanDoneMsg:
		m.allResults = msg.result.Packages
		m.results = msg.result.Packages
		m.scanErrors = msg.result.Errors
		m.truncated = msg.result.Truncated
		m.filter = m.sourceFilter
		m.cursor = 0
		if m.filter != "" {
			m.applyFilter()
		}
		if len(m.results) == 0 {
			m.state = stateDone
			return m, nil
		}
		m.state = stateResults
		return m, nil
	case catalogDoneMsg:
		m.catalog = msg.result.Packages
		m.catalogErrors = msg.result.Errors
		m.catalogLoaded = true
		m.catalogLoading = false
		if m.state == stateScanning {
			m.searchFromCatalog(m.searchTerm)
			return m, nil
		}
		m.refreshSuggestions()
		return m, nil
	case uninstallDoneMsg:
		m.err = msg.err
		m.state = stateDone
		// Refresh the installed inventory after every uninstall attempt so a
		// removed package cannot remain selectable from the cached catalog.
		m.catalogLoading = true
		m.catalogLoaded = false
		m.catalog = nil
		m.catalogErrors = nil
		m.suggestions = nil
		return m, doScanInstalled()
	case sudoAuthDoneMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("sudo authentication failed: %w", msg.err)
			m.state = statePassword
			m.passwordInput.SetValue("")
			m.passwordInput.Focus()
			return m, tea.ClearScreen
		}
		m.state = stateExecuting
		return m, tea.Batch(tea.ClearScreen, m.spinner.Tick, doUninstall(m.selected, m.mode, m.confirmDataDirs, m.confirmDesktopFiles))
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
	case historyMsg:
		m.history = msg.history
		m.historyErr = msg.err
		m.historyCursor = 0
		return m, nil
	case confirmDepsMsg:
		m.confirmDeps = msg.deps
		m.confirmDepsKnown = msg.depsKnown
		m.confirmDepsErr = msg.depsErr
		m.confirmRemovals = msg.removals
		m.confirmRemovalErr = msg.removalErr
		m.confirmDataDirs = msg.dataDirs
		m.confirmDesktopFiles = msg.desktopFiles
		m.confirmDepsReady = true
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
	case stateFinalConfirm:
		return m.updateFinalConfirm(msg)
	case statePassword:
		return m.updatePassword(msg)
	case stateHelp:
		return m.updateHelp(msg)
	case statePackageInfo:
		return m.updatePackageInfo(msg)
	case stateDryRun:
		return m.updateDryRun(msg)
	case stateHistory:
		return m.updateHistory(msg)
	case stateDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func (m Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "f5", "ctrl+r":
			if m.catalogLoading {
				return m, nil
			}
			m.catalogLoading = true
			m.catalogLoaded = false
			m.catalog = nil
			m.catalogErrors = nil
			m.suggestions = nil
			m.suggestionCursor = 0
			return m, doScanInstalled()
		case "enter":
			if len(m.suggestions) > 0 && m.suggestionCursor >= 0 && m.suggestionCursor < len(m.suggestions) {
				m.searchTerm = strings.TrimSpace(m.textInput.Value())
				m.err = nil
				m.searchFromCatalog(m.searchTerm)
				for i, pkg := range m.results {
					if samePackage(pkg, m.suggestions[m.suggestionCursor]) {
						m.cursor = i
						break
					}
				}
				return m, nil
			}
			term := strings.TrimSpace(m.textInput.Value())
			if term == "" {
				return m, nil
			}
			if len([]rune(term)) < minSearchLen {
				m.err = fmt.Errorf("la búsqueda debe tener al menos %d caracteres", minSearchLen)
				return m, nil
			}
			m.err = nil
			m.searchTerm = term
			m.filter = ""
			m.state = stateScanning
			if m.catalogLoaded {
				m.searchFromCatalog(term)
				return m, nil
			}
			return m, m.spinner.Tick
		case "up", "down":
			if len(m.suggestions) > 0 {
				if key.String() == "up" && m.suggestionCursor > 0 {
					m.suggestionCursor--
				}
				if key.String() == "down" && m.suggestionCursor < len(m.suggestions)-1 {
					m.suggestionCursor++
				}
				return m, nil
			}
		case "tab":
			if len(m.suggestions) > 0 {
				pkg := m.suggestions[m.suggestionCursor]
				m.textInput.SetValue(pkg.Name)
				m.refreshSuggestions()
				return m, nil
			}
		case "esc":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	m.refreshSuggestions()
	return m, cmd
}

func (m *Model) refreshSuggestions() {
	term := strings.TrimSpace(m.textInput.Value())
	m.suggestions = nil
	m.suggestionCursor = 0
	if !m.catalogLoaded || term == "" {
		return
	}
	type ranked struct {
		pkg  PackageResult
		rank int
	}
	var matches []ranked
	query := strings.ToLower(term)
	for _, pkg := range m.catalog {
		fields := []string{pkg.Name, pkg.ID, pkg.Installation, pkg.Path, pkg.Source}
		text := strings.ToLower(strings.Join(fields, " "))
		var rank int
		switch {
		case strings.EqualFold(pkg.Name, term) || strings.EqualFold(pkg.ID, term):
			rank = 0
		case strings.HasPrefix(strings.ToLower(pkg.Name), query) || strings.HasPrefix(strings.ToLower(pkg.ID), query):
			rank = 1
		case strings.Contains(text, query):
			rank = 2
		case textMatches(text, term):
			rank = 3
		default:
			continue
		}
		matches = append(matches, ranked{pkg: pkg, rank: rank})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		return strings.ToLower(matches[i].pkg.Name) < strings.ToLower(matches[j].pkg.Name)
	})
	for i := 0; i < len(matches) && i < 8; i++ {
		m.suggestions = append(m.suggestions, matches[i].pkg)
	}
}

func samePackage(a, b PackageResult) bool {
	return a.Source == b.Source && a.Name == b.Name && a.ID == b.ID && a.Path == b.Path
}

func (m *Model) searchFromCatalog(term string) {
	m.searchTerm = term
	var found []PackageResult
	for _, pkg := range m.catalog {
		if textMatches(strings.Join([]string{pkg.Name, pkg.ID, pkg.Installation, pkg.Path}, " "), term) {
			found = append(found, pkg)
		}
	}
	sortPackages(found)
	counts := map[string]int{}
	m.allResults = nil
	m.truncated = false
	for _, pkg := range found {
		if counts[pkg.Source] >= maxScanResults {
			m.truncated = true
			continue
		}
		counts[pkg.Source]++
		m.allResults = append(m.allResults, pkg)
	}
	m.scanErrors = m.catalogErrors
	m.filter = m.sourceFilter
	m.results = m.allResults
	m.cursor = 0
	if m.filter != "" {
		m.applyFilter()
	}
	if len(m.results) == 0 {
		m.state = stateDone
	} else {
		m.state = stateResults
	}
}

func (m Model) updateSpinner(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) updateResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		pageSize := m.resultsPageSize()
		switch key.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
		case "pgup":
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "pgdown":
			m.cursor += pageSize
			if m.cursor >= len(m.results) {
				m.cursor = len(m.results) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "enter":
			if len(m.results) > 0 {
				m.selected = m.results[m.cursor]
				if m.selected.Source == SourceWebApp {
					m.mode = modeNormal
					m.confirmDeps = nil
					m.confirmDepsKnown = false
					m.confirmDepsErr = ""
					m.confirmRemovals = nil
					m.confirmRemovalErr = ""
					m.confirmDataDirs = nil
					m.confirmDesktopFiles = nil
					m.confirmDepsReady = true
					m.state = stateConfirm
					return m, nil
				}
				m.state = stateModeSelect
				m.modeCursor = 0
			}
		case "i":
			if len(m.results) > 0 {
				m.selected = m.results[m.cursor]
				m.pkgInfo = nil
				m.state = statePackageInfo
				return m, tea.Batch(m.spinner.Tick, doGetPackageInfo(m.selected))
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
		case "h":
			m.state = stateHistory
			m.history = History{}
			m.historyErr = nil
			m.historyCursor = 0
			return m, doLoadHistory()
		case "esc":
			m.state = stateInput
			m.textInput.Focus()
			m.searchTerm = ""
			m.textInput.SetValue("")
			m.refreshSuggestions()
			m.filter = ""
			m.truncated = false
			m.err = nil
		case "1":
			m.filter = ""
			m.results = m.allResults
			m.cursor = 0
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
		case "6":
			m.filter = SourcePacman
			m.applyFilter()
		case "7":
			m.filter = SourceWebApp
			m.applyFilter()
		}
	}
	return m, nil
}

func (m Model) resultsPageSize() int {
	if m.height <= 0 {
		return resultsPageSize
	}
	pageSize := m.height - 12
	if pageSize < 1 {
		return 1
	}
	if pageSize > resultsPageSize {
		return resultsPageSize
	}
	return pageSize
}

func (m Model) historyPageSize() int {
	if m.height <= 0 {
		return 4
	}
	pageSize := (m.height - 10) / 3
	if pageSize < 1 {
		return 1
	}
	return pageSize
}

func (m *Model) applyFilter() {
	if m.filter == "" {
		m.results = m.allResults
		m.cursor = 0
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

// visibleWindow returns the slice of results to render and the absolute start index.
func visibleWindow(total, cursor, pageSize int) (start, end int) {
	if total <= 0 || pageSize <= 0 {
		return 0, 0
	}
	if total <= pageSize {
		return 0, total
	}
	start = cursor - pageSize/2
	if start < 0 {
		start = 0
	}
	end = start + pageSize
	if end > total {
		end = total
		start = end - pageSize
		if start < 0 {
			start = 0
		}
	}
	return start, end
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
				m.confirmDeps = nil
				m.confirmDepsKnown = false
				m.confirmDepsErr = ""
				m.confirmRemovals = nil
				m.confirmRemovalErr = ""
				m.confirmDataDirs = nil
				m.confirmDesktopFiles = nil
				m.confirmDepsReady = false
				m.state = stateConfirm
				return m, tea.Batch(m.spinner.Tick, doLoadConfirmDeps(m.selected, m.mode))
			case 1:
				m.mode = modeComplete
				m.confirmDeps = nil
				m.confirmDepsKnown = false
				m.confirmDepsErr = ""
				m.confirmRemovals = nil
				m.confirmRemovalErr = ""
				m.confirmDataDirs = nil
				m.confirmDesktopFiles = nil
				m.confirmDepsReady = false
				m.state = stateConfirm
				return m, tea.Batch(m.spinner.Tick, doLoadConfirmDeps(m.selected, m.mode))
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
	// Keep spinner alive while deps load
	if !m.confirmDepsReady {
		if _, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch strings.ToLower(key.String()) {
		case "s", "y":
			if !m.confirmDepsReady {
				return m, nil
			}
			if m.confirmDepsErr != "" && (m.selected.Source == SourceAPT || m.selected.Source == SourcePacman) {
				return m, nil
			}
			if m.mode == modeComplete && (m.selected.Source == SourceAPT || m.selected.Source == SourcePacman) && m.confirmRemovalErr != "" {
				return m, nil
			}
			if m.requiresFinalConfirmation() {
				m.finalConfirmInput.SetValue("")
				m.finalConfirmInput.Focus()
				m.finalConfirmErr = ""
				m.finalConfirmPage = 0
				m.state = stateFinalConfirm
				return m, tea.ClearScreen
			}
			return m.beginUninstall()
		case "r":
			m.confirmDeps = nil
			m.confirmDepsKnown = false
			m.confirmDepsErr = ""
			m.confirmRemovals = nil
			m.confirmRemovalErr = ""
			m.confirmDataDirs = nil
			m.confirmDesktopFiles = nil
			m.confirmDepsReady = false
			return m, tea.Batch(m.spinner.Tick, doLoadConfirmDeps(m.selected, m.mode))
		case "n", "esc":
			m.state = stateModeSelect
			m.confirmDeps = nil
			m.confirmDepsKnown = false
			m.confirmDepsErr = ""
			m.confirmRemovals = nil
			m.confirmRemovalErr = ""
			m.confirmDataDirs = nil
			m.confirmDesktopFiles = nil
			m.confirmDepsReady = false
		}
	}
	return m, nil
}

func (m Model) updateFinalConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "pgup":
			if m.finalConfirmPage > 0 {
				m.finalConfirmPage--
			}
			return m, nil
		case "pgdown":
			lastPage := (len(m.finalConfirmItems()) - 1) / m.finalConfirmPageSize()
			if m.finalConfirmPage < lastPage {
				m.finalConfirmPage++
			}
			return m, nil
		case "esc":
			m.state = stateConfirm
			m.finalConfirmInput.SetValue("")
			m.finalConfirmErr = ""
			m.finalConfirmPage = 0
			return m, tea.ClearScreen
		case "enter":
			if m.finalConfirmInput.Value() != m.selected.Name {
				m.finalConfirmErr = "El nombre no coincide exactamente; no se ejecutará la desinstalación."
				return m, nil
			}
			m.finalConfirmErr = ""
			return m.beginUninstall()
		}
	}
	previousValue := m.finalConfirmInput.Value()
	var cmd tea.Cmd
	m.finalConfirmInput, cmd = m.finalConfirmInput.Update(msg)
	if m.finalConfirmInput.Value() != previousValue {
		m.finalConfirmErr = ""
	}
	return m, cmd
}

func (m Model) requiresFinalConfirmation() bool {
	return len(m.confirmDeps) > 0 || len(m.additionalRemovalPackages()) > 0
}

func (m Model) finalConfirmItems() []string {
	items := make([]string, 0, len(m.confirmDeps)+len(m.confirmRemovals)+len(m.confirmDataDirs)+len(m.confirmDesktopFiles))
	for _, name := range m.confirmDeps {
		items = append(items, "Dependiente: "+safeDisplayText(name))
	}
	for _, name := range m.additionalRemovalPackages() {
		items = append(items, "Se quitará: "+safeDisplayText(name))
	}
	for _, dir := range m.confirmDataDirs {
		items = append(items, "Se eliminarán datos: "+safeDisplayText(dir))
	}
	for _, file := range m.confirmDesktopFiles {
		items = append(items, "Se quitará acceso del menú: "+safeDisplayText(file))
	}
	return items
}

func (m Model) finalConfirmPageSize() int {
	if m.height <= 0 {
		return 6
	}
	pageSize := m.height - 18
	if pageSize < 1 {
		return 1
	}
	if pageSize > 8 {
		return 8
	}
	return pageSize
}

func (m Model) additionalRemovalPackages() []string {
	var additional []string
	seen := make(map[string]bool, len(m.confirmRemovals))
	for _, name := range m.confirmRemovals {
		baseName, _, _ := strings.Cut(name, ":") // APT may include an architecture suffix.
		if baseName == m.selected.Name || seen[name] {
			continue
		}
		seen[name] = true
		additional = append(additional, name)
	}
	return additional
}

func (m Model) beginUninstall() (tea.Model, tea.Cmd) {
	if needsPrivilege(m.selected) {
		m.state = statePassword
		m.err = nil
		m.passwordInput.SetValue("")
		m.passwordInput.Focus()
		return m, tea.ClearScreen
	}
	m.state = stateExecuting
	return m, tea.Batch(tea.ClearScreen, m.spinner.Tick, doUninstall(m.selected, m.mode, m.confirmDataDirs, m.confirmDesktopFiles))
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
			m.state = m.helpReturnState
			return m, tea.ClearScreen
		}
	}
	return m, nil
}

func (m Model) updatePackageInfo(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.pkgInfo == nil {
		if _, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q", "enter", "i":
			m.state = stateResults
		}
	}
	return m, nil
}

func (m Model) updateDryRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dryResult == nil {
		if _, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q", "enter", "d":
			m.state = stateResults
		}
	}
	return m, nil
}

func (m Model) updateHistory(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.historyCursor > 0 {
				m.historyCursor--
			}
		case "down", "j":
			if m.historyCursor < len(m.history.Entries)-1 {
				m.historyCursor++
			}
		case "pgup":
			m.historyCursor -= m.historyPageSize()
			if m.historyCursor < 0 {
				m.historyCursor = 0
			}
		case "pgdown":
			m.historyCursor += m.historyPageSize()
			if m.historyCursor >= len(m.history.Entries) {
				m.historyCursor = len(m.history.Entries) - 1
			}
			if m.historyCursor < 0 {
				m.historyCursor = 0
			}
		case "esc", "q", "h":
			m.state = stateResults
			return m, tea.ClearScreen
		}
	}
	return m, nil
}

func (m Model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		_ = key
		m.state = stateInput
		m.textInput.Focus()
		m.textInput.SetValue("")
		m.refreshSuggestions()
		m.err = nil
		m.results = nil
		m.allResults = nil
		m.filter = ""
		m.truncated = false
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return dimStyle.Render("Operación cancelada.") + "\n"
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
	case stateFinalConfirm:
		return m.viewFinalConfirm() + separator
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
	case stateHistory:
		return m.viewHistory() + separator
	}
	return ""
}

func (m Model) viewInput() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Desinstalador interactivo"))
	b.WriteString("\n\n")
	b.WriteString(sectionStyle.Render("¿Qué programa quieres buscar?"))
	b.WriteString("\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	if m.catalogLoading {
		b.WriteString(dimStyle.Render("Cargando aplicaciones instaladas para sugerencias…") + "\n\n")
	} else if len(m.suggestions) > 0 {
		b.WriteString(sectionStyle.Render("Sugerencias") + "\n")
		for i, result := range m.suggestions {
			b.WriteString("  " + formatResult(result, i == m.suggestionCursor) + "\n")
		}
		b.WriteString(dimStyle.Render("↑/↓ elegir · Tab completar · Enter buscar") + "\n\n")
	}
	if !m.catalogLoading && len(m.catalogErrors) > 0 {
		b.WriteString(warningTextStyle.Render("Algunas fuentes no respondieron; las sugerencias pueden estar incompletas.") + "\n\n")
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render("✗ "+safeDisplayText(m.err.Error())) + "\n\n")
	}
	b.WriteString(keyHint("f5", "actualizar apps") + "  " + keyHint("esc", "salir") + "  " + keyHint("?", "ayuda") + "  " + dimStyle.Render("mínimo 2 caracteres"))
	return b.String()
}

func (m Model) viewScanning() string {
	return fmt.Sprintf("%s\n\n%s Buscando \"%s\" en el sistema…\n",
		renderBanner(),
		m.spinner.View(),
		safeDisplayText(m.searchTerm),
	)
}

func (m Model) viewResults() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%d resultados para \"%s\"", len(m.results), safeDisplayText(m.searchTerm))))
	b.WriteString(" ")
	if m.filter == "" {
		b.WriteString(sourceTag("ALL").Render("[Todas las fuentes]"))
	} else {
		b.WriteString(sourceTag(m.filter).Render("[Solo " + m.filter + "]"))
	}
	if m.truncated {
		b.WriteString(" ")
		b.WriteString(warningTextStyle.Render(fmt.Sprintf("(máximo %d por fuente)", maxScanResults)))
	}
	b.WriteString("\n\n")

	start, end := visibleWindow(len(m.results), m.cursor, m.resultsPageSize())
	if start > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d resultados anteriores\n", start)))
	}
	for i := start; i < end; i++ {
		r := m.results[i]
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("▸ ")
		}
		b.WriteString(cursor + formatResult(r, i == m.cursor) + "\n")
	}
	if end < len(m.results) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d resultados siguientes\n", len(m.results)-end)))
	}

	b.WriteString("\n")
	if len(m.results) > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Seleccionado %d de %d", m.cursor+1, len(m.results))) + "\n")
	}
	if len(m.scanErrors) > 0 {
		for _, e := range m.scanErrors {
			b.WriteString(warningTextStyle.Render("⚠ "+safeDisplayText(e.Error())) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(keyHint("↑/↓", "moverse") + "  " + keyHint("PgUp/PgDn", "página") + "  " + keyHint("1-7", "filtrar") + "\n")
	b.WriteString(keyHint("Enter", "elegir") + "  " + keyHint("i", "detalles") + "  " + keyHint("d", "simular") + "  " + keyHint("h", "historial") + "  " + keyHint("?", "ayuda") + "  " + keyHint("Esc", "buscar otra vez"))
	return b.String()
}

func (m Model) viewModeSelect() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n")
	b.WriteString(panelStyle.Render(fmt.Sprintf("Programa: %s  %s", selectedStyle.Render(safeDisplayText(m.selected.Name)), sourceTag(m.selected.Source).Render("["+m.selected.Source+"]"))))
	b.WriteString("\n\n")
	b.WriteString(subtitleStyle.Render("Elige cómo quieres desinstalarlo"))
	b.WriteString("\n\n")

	options := []string{
		"Normal     Quitar el programa y conservar la configuración",
		"Completa   Quitar el programa, los datos y accesos indicados",
		"Volver a los resultados",
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
	b.WriteString(keyHint("↑/↓", "moverse") + "  " + keyHint("Enter", "continuar") + "  " + keyHint("Esc", "volver"))
	return b.String()
}

func (m Model) viewConfirm() string {
	if !m.confirmDepsReady {
		return fmt.Sprintf("%s\n\n%s Revisando dependencias…\n",
			renderBanner(),
			m.spinner.View(),
		)
	}

	modeText := "Normal"
	if m.mode == modeComplete {
		modeText = "Completa"
	}
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n\n")
	alert := fmt.Sprintf("¿Desinstalar %q?\nFuente: %s  ·  Modo: %s",
		safeDisplayText(m.selected.Name), m.selected.Source, modeText)
	b.WriteString(warningStyle.Render(alert))
	b.WriteString("\n\n")

	if len(m.confirmDeps) > 0 {
		maxShow := 12
		b.WriteString(errorStyle.Render(fmt.Sprintf(
			"⚠ %d paquete(s) dependen de este y podrían dejar de funcionar:",
			len(m.confirmDeps),
		)))
		b.WriteString("\n")
		for i, dep := range m.confirmDeps {
			if i >= maxShow {
				b.WriteString(dimStyle.Render(fmt.Sprintf("    … y %d más\n", len(m.confirmDeps)-maxShow)))
				break
			}
			b.WriteString("    - " + safeDisplayText(dep) + "\n")
		}
		b.WriteString("\n")
	} else if m.confirmDepsErr != "" {
		b.WriteString(errorStyle.Render("⚠ No se pudieron comprobar las dependencias: "+safeDisplayText(m.confirmDepsErr)) + "\n\n")
	} else if m.confirmDepsKnown {
		b.WriteString(dimStyle.Render("No se encontraron paquetes que dependan de este.") + "\n\n")
	} else {
		b.WriteString(dimStyle.Render("Esta fuente no permite comprobar dependencias inversas.") + "\n\n")
	}

	if m.mode == modeComplete && (m.selected.Source == SourceAPT || m.selected.Source == SourcePacman) {
		command := "apt purge --autoremove"
		if m.selected.Source == SourcePacman {
			command = "pacman -Rns"
		}
		b.WriteString(warningStyle.Render("La desinstalación completa ejecutará "+command+" y puede quitar dependencias") + "\n")
		if m.confirmRemovalErr != "" {
			b.WriteString(errorStyle.Render("No se pudo previsualizar lo que se quitará: "+safeDisplayText(m.confirmRemovalErr)) + "\n")
			b.WriteString(errorStyle.Render("La desinstalación completa queda bloqueada hasta obtener la vista previa.") + "\n")
		} else if len(m.additionalRemovalPackages()) == 0 {
			b.WriteString(dimStyle.Render(m.selected.Source+" no indicó paquetes adicionales para quitar.") + "\n")
		} else {
			removals := m.additionalRemovalPackages()
			b.WriteString("También se quitarán " + fmt.Sprint(len(removals)) + " paquete(s):\n")
			for i, name := range removals {
				if i >= 12 {
					b.WriteString(dimStyle.Render(fmt.Sprintf("    … y %d más\n", len(removals)-12)))
					break
				}
				b.WriteString("    - " + safeDisplayText(name) + "\n")
			}
		}
		b.WriteString("\n")
	}
	if m.selected.Source == SourceFlatpak {
		b.WriteString(dimStyle.Render("Instalación: "+safeDisplayText(m.selected.Installation)) + "\n")
		if m.mode == modeComplete {
			b.WriteString(dimStyle.Render("Flatpak también ejecutará --delete-data para los datos de la aplicación.") + "\n")
		}
		b.WriteString("\n")
	}
	if m.selected.Source == SourceWebApp {
		b.WriteString(warningStyle.Render("Se eliminará este acceso del menú:") + "\n")
		b.WriteString("    - " + safeDisplayText(m.selected.Path) + "\n")
		b.WriteString(dimStyle.Render("La web y los datos del navegador se conservarán.") + "\n\n")
	}
	if m.selected.Source == SourceSnap {
		if m.mode == modeComplete {
			b.WriteString(warningTextStyle.Render("Snap no creará una nueva copia automática de datos; las copias anteriores no se borran.") + "\n\n")
		} else {
			b.WriteString(dimStyle.Render("Snap puede crear una copia automática de datos al desinstalar.") + "\n\n")
		}
	}
	if m.mode == modeComplete && len(m.confirmDataDirs) > 0 {
		b.WriteString(warningStyle.Render("Se eliminarán estas carpetas de datos:") + "\n")
		for _, dir := range m.confirmDataDirs {
			b.WriteString("    - " + safeDisplayText(dir) + "\n")
		}
		b.WriteString("\n")
	}
	if m.mode == modeComplete && len(m.confirmDesktopFiles) > 0 {
		b.WriteString(warningStyle.Render("También se eliminarán estos accesos del menú:") + "\n")
		for _, file := range m.confirmDesktopFiles {
			b.WriteString("    - " + safeDisplayText(file) + "\n")
		}
		b.WriteString("\n")
	}
	if m.mode == modeComplete && m.selected.Source == SourceAppImage && len(m.confirmDataDirs) == 0 && len(m.confirmDesktopFiles) == 0 {
		b.WriteString(dimStyle.Render("No se detectaron carpetas de datos ni accesos de menú asociados.") + "\n\n")
	}

	blocked := (m.confirmDepsErr != "" && (m.selected.Source == SourceAPT || m.selected.Source == SourcePacman)) ||
		(m.mode == modeComplete && (m.selected.Source == SourceAPT || m.selected.Source == SourcePacman) && m.confirmRemovalErr != "")
	if blocked {
		b.WriteString(warningTextStyle.Render("La confirmación está bloqueada; revisa el aviso anterior.") + "\n")
		b.WriteString(keyHint("r", "reintentar comprobaciones") + "  " + keyHint("n / Esc", "cancelar"))
	} else {
		b.WriteString(keyHint("s / y", "confirmar") + "  " + keyHint("n / Esc", "cancelar"))
	}
	return b.String()
}

func (m Model) viewFinalConfirm() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n\n")
	b.WriteString(errorStyle.Render("⚠ Confirmación adicional requerida"))
	b.WriteString("\n\n")
	b.WriteString("Para evitar una eliminación accidental, escribe exactamente el nombre del programa:\n")
	b.WriteString("Programa: " + selectedStyle.Render(safeDisplayText(m.selected.Name)) + "\n\n")
	b.WriteString("Fuente: " + safeDisplayText(m.selected.Source) + " · Modo: ")
	if m.mode == modeComplete {
		b.WriteString("Completa\n\n")
	} else {
		b.WriteString("Normal\n\n")
	}
	items := m.finalConfirmItems()
	pageSize := m.finalConfirmPageSize()
	pages := (len(items) + pageSize - 1) / pageSize
	page := m.finalConfirmPage
	if pages == 0 {
		page = 0
	} else if page >= pages {
		page = pages - 1
	}
	start := page * pageSize
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	b.WriteString("Impacto adicional: " + fmt.Sprint(len(items)) + " elemento(s)")
	if pages > 1 {
		b.WriteString(" · página " + fmt.Sprint(page+1) + " de " + fmt.Sprint(pages))
	}
	b.WriteString(":\n")
	for _, item := range items[start:end] {
		b.WriteString("    - " + item + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.finalConfirmInput.View())
	b.WriteString("\n\n")
	if m.finalConfirmErr != "" {
		b.WriteString(errorStyle.Render(safeDisplayText(m.finalConfirmErr)) + "\n\n")
	}
	if pages > 1 {
		b.WriteString(keyHint("PgUp/PgDn", "revisar impacto") + "  ")
	}
	b.WriteString(keyHint("Enter", "confirmar escribiendo el nombre exacto") + "  " + keyHint("Esc", "volver sin desinstalar"))
	return b.String()
}

func (m Model) viewPassword() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n\n")
	b.WriteString("Programa: " + selectedStyle.Render(safeDisplayText(m.selected.Name)) + "\n\n")
	b.WriteString(subtitleStyle.Render("Se necesita tu contraseña de sudo para desinstalarlo:"))
	b.WriteString("\n\n")
	b.WriteString(m.passwordInput.View())
	b.WriteString("\n\n")
	if m.err != nil {
		b.WriteString(errorStyle.Render("✗ "+safeDisplayText(m.err.Error())) + "\n\n")
	}
	b.WriteString(keyHint("Enter", "validar") + "  " + keyHint("Esc", "cancelar"))
	return b.String()
}

func (m Model) viewSudoAuth() string {
	return fmt.Sprintf("%s\n\n%s Verificando contraseña…\n",
		renderBanner(),
		m.spinner.View(),
	)
}

func (m Model) viewExecuting() string {
	return fmt.Sprintf("%s\n\n%s Desinstalando %s…\n",
		renderBanner(),
		m.spinner.View(),
		safeDisplayText(m.selected.Name),
	)
}

func (m Model) viewDone() string {
	if len(m.results) == 0 && m.err == nil {
		if len(m.scanErrors) > 0 {
			var b strings.Builder
			b.WriteString(renderBanner())
			b.WriteString("\n\n")
			b.WriteString(errorStyle.Render("No se pudieron confirmar resultados porque algunas fuentes fallaron:") + "\n")
			for _, scanErr := range m.scanErrors {
				b.WriteString(warningTextStyle.Render("  ⚠ "+safeDisplayText(scanErr.Error())) + "\n")
			}
			b.WriteString("\n" + dimStyle.Render("Pulsa cualquier tecla para continuar."))
			return b.String()
		}
		return fmt.Sprintf("%s\n\n%s\n\n%s\n",
			renderBanner(),
			dimStyle.Render("No se encontraron resultados para \""+safeDisplayText(m.searchTerm)+"\"."),
			dimStyle.Render("Pulsa cualquier tecla para continuar."),
		)
	}
	if m.err != nil {
		return fmt.Sprintf("%s\n\n%s\n%s\n\n%s\n",
			renderBanner(),
			errorStyle.Render("✗ Error durante la desinstalación:"),
			errorStyle.Render(safeDisplayText(m.err.Error())),
			dimStyle.Render("Pulsa cualquier tecla para continuar."),
		)
	}
	if m.selected.Source == SourceWebApp {
		return fmt.Sprintf("%s\n\n%s\n\n%s\n",
			renderBanner(),
			successStyle.Render("✓ Se quitó el acceso web \""+safeDisplayText(m.selected.Name)+"\"."),
			dimStyle.Render("Pulsa cualquier tecla para continuar."),
		)
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s\n",
		renderBanner(),
		successStyle.Render("✓ Se desinstaló correctamente \""+safeDisplayText(m.selected.Name)+"\"."),
		dimStyle.Render("Pulsa cualquier tecla para continuar."),
	)
}

func (m Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Ayuda"))
	b.WriteString("\n\n")
	b.WriteString(sectionStyle.Render("Navegación"))
	b.WriteString("\n")
	b.WriteString("  ↑/k       Subir\n")
	b.WriteString("  ↓/j       Bajar\n")
	b.WriteString("  PgUp/PgDn Cambiar de página\n")
	b.WriteString("  Enter     Elegir / confirmar\n")
	b.WriteString("  Esc       Volver / salir\n")
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Acciones en resultados"))
	b.WriteString("\n")
	b.WriteString("  i  Ver información del paquete\n")
	b.WriteString("  d  Simular la desinstalación\n")
	b.WriteString("  h  Consultar el historial\n")
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Filtros en resultados"))
	b.WriteString("\n")
	b.WriteString("  1  Todas las fuentes\n")
	b.WriteString("  2  Solo APT\n")
	b.WriteString("  3  Solo Flatpak\n")
	b.WriteString("  4  Solo Snap\n")
	b.WriteString("  5  Solo AppImage\n")
	b.WriteString("  6  Solo Pacman\n")
	b.WriteString("  7  Solo lanzadores web de Omarchy\n")
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Modos de desinstalación"))
	b.WriteString("\n")
	b.WriteString("  Normal:    Quita el programa y conserva la configuración\n")
	b.WriteString("  Completa: Quita el programa, las rutas de datos y accesos listados\n")
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Pulsa Esc o ? para volver a la pantalla anterior."))
	return b.String()
}

func (m Model) viewHistory() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Historial de desinstalaciones"))
	b.WriteString("\n\n")
	if m.historyErr != nil {
		b.WriteString(errorStyle.Render("No se pudo leer el historial: " + safeDisplayText(m.historyErr.Error())))
	} else if len(m.history.Entries) == 0 {
		b.WriteString(dimStyle.Render("Todavía no hay operaciones registradas."))
	} else {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Últimas %d operaciones · más recientes primero", len(m.history.Entries))))
		b.WriteString("\n\n")
		start, end := visibleWindow(len(m.history.Entries), m.historyCursor, m.historyPageSize())
		if start > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d operaciones anteriores\n", start)))
		}
		for i := start; i < end; i++ {
			entry := m.history.Entries[len(m.history.Entries)-1-i]
			marker := successStyle.Render("✓")
			if !entry.Success {
				marker = errorStyle.Render("✗")
			}
			mode := "Normal"
			if entry.Mode == modeComplete {
				mode = "Completa"
			}
			name := truncateDisplayText(safeDisplayText(entry.Name), 64)
			b.WriteString(marker + " " + entry.Timestamp.Format("2006-01-02 15:04") + "  " + selectedStyle.Render(name) + "\n")
			b.WriteString("    " + safeDisplayText(entry.Source) + " · " + mode + " · " + entry.Duration.Round(100*time.Millisecond).String() + "\n")
			if !entry.Success && entry.Error != "" {
				b.WriteString("    " + errorStyle.Render(truncateDisplayText(safeDisplayText(entry.Error), 96)) + "\n")
			}
		}
		if end < len(m.history.Entries) {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d operaciones siguientes\n", len(m.history.Entries)-end)))
		}
	}
	b.WriteString("\n" + keyHint("↑/↓", "moverse") + "  " + keyHint("PgUp/PgDn", "página") + "  " + keyHint("Esc / q / h", "volver a resultados"))
	return b.String()
}

func (m Model) viewPackageInfo() string {
	if m.pkgInfo == nil {
		return fmt.Sprintf("%s\n\n%s Cargando información…\n",
			renderBanner(),
			m.spinner.View(),
		)
	}
	info := m.pkgInfo
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Información del paquete"))
	b.WriteString("\n\n")
	b.WriteString("  Nombre:       " + selectedStyle.Render(safeDisplayText(info.Name)) + "\n")
	if info.Version != "" {
		b.WriteString("  Versión:      " + safeDisplayText(info.Version) + "\n")
	}
	if info.InstalledSize != "" {
		b.WriteString("  Tamaño:       " + safeDisplayText(info.InstalledSize) + "\n")
	}
	if info.Description != "" {
		maxWidth := m.width - 16 // "  Descripción: " = 15 + 1 espacio
		if maxWidth < 30 {
			maxWidth = 60
		}
		wrapped := wordWrap(safeDisplayText(info.Description), maxWidth)
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			if i == 0 {
				b.WriteString("  Descripción:  " + line + "\n")
			} else {
				b.WriteString("               " + line + "\n")
			}
		}
	}
	if len(info.Dependents) > 0 {
		b.WriteString("\n")
		b.WriteString(highlightStyle.Render("  ⚠ Paquetes que dependen de este ("+fmt.Sprintf("%d", len(info.Dependents))+"):") + "\n")
		for i, dep := range info.Dependents {
			if i == 12 {
				b.WriteString(dimStyle.Render(fmt.Sprintf("    … y %d más\n", len(info.Dependents)-12)))
				break
			}
			b.WriteString("    - " + safeDisplayText(dep) + "\n")
		}
	} else if info.DependentsError != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  No se pudieron comprobar dependencias: "+safeDisplayText(info.DependentsError)) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(keyHint("Esc / Enter / i", "volver a resultados"))
	return b.String()
}

func (m Model) viewDryRun() string {
	if m.dryResult == nil {
		return fmt.Sprintf("%s\n\n%s Preparando simulación…\n",
			renderBanner(),
			m.spinner.View(),
		)
	}
	result := m.dryResult

	var b strings.Builder
	b.WriteString(renderBannerSuffix("Simulación de desinstalación"))
	b.WriteString("\n\n")
	b.WriteString("  Programa: " + selectedStyle.Render(safeDisplayText(m.selected.Name)) + " [" + m.selected.Source + "]\n")
	modeText := "Normal"
	if m.mode == modeComplete {
		modeText = "Completa (purga)"
	}
	b.WriteString("  Modo:    " + modeText + "\n")
	b.WriteString("  Tamaño estimado del paquete (sin dependencias ni datos): " + successStyle.Render(FormatBytes(result.EstimateBytes)) + "\n")
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Acciones previstas"))
	b.WriteString("\n")
	if len(result.PackagesToRemove) > 0 || result.RemovalPreviewErr != "" {
		if result.RemovalPreviewErr != "" {
			b.WriteString(warningTextStyle.Render("No se pudo obtener la lista de paquetes adicionales: "+safeDisplayText(result.RemovalPreviewErr)) + "\n")
		} else {
			b.WriteString("Paquetes que se quitarían además: " + fmt.Sprint(len(result.PackagesToRemove)) + "\n")
			for i, name := range result.PackagesToRemove {
				if i == 12 {
					b.WriteString(dimStyle.Render(fmt.Sprintf("  … y %d más\n", len(result.PackagesToRemove)-12)))
					break
				}
				b.WriteString("  - " + safeDisplayText(name) + "\n")
			}
		}
	}
	for _, cmd := range result.Commands {
		b.WriteString("  $ " + safeDisplayText(cmd) + "\n")
	}
	if len(result.DirsToRemove) > 0 {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render("Carpetas que se eliminarían"))
		b.WriteString("\n")
		for _, dir := range result.DirsToRemove {
			b.WriteString("  - " + safeDisplayText(dir) + "\n")
		}
	}
	if len(result.FilesToRemove) > 0 {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render("Accesos del menú que se eliminarían"))
		b.WriteString("\n")
		for _, file := range result.FilesToRemove {
			b.WriteString("  - " + safeDisplayText(file) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(keyHint("Esc / Enter / d", "volver a resultados"))
	return b.String()
}

func sourceTag(source string) lipgloss.Style {
	switch source {
	case SourceAPT:
		return sourceAPT
	case SourcePacman:
		return sourcePacman
	case SourceFlatpak:
		return sourceFlatpak
	case SourceSnap:
		return sourceSnap
	case SourceAppImage:
		return sourceAppImage
	case SourceWebApp:
		return sourceWebApp
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
	case SourcePacman:
		tag = sourcePacman.Render("[Pacman]")
	case SourceFlatpak:
		tag = sourceFlatpak.Render("[Flatpak]")
	case SourceSnap:
		tag = sourceSnap.Render("[Snap]")
	case SourceAppImage:
		tag = sourceAppImage.Render("[AppImage]")
	case SourceWebApp:
		tag = sourceWebApp.Render("[Web App]")
	}

	name := safeDisplayText(r.Name)
	if r.Source == SourceFlatpak && r.ID != "" {
		name = fmt.Sprintf("%s (%s, %s)", name, safeDisplayText(r.ID), safeDisplayText(r.Installation))
	}
	if r.Source == SourceAppImage && r.Path != "" {
		name = fmt.Sprintf("%s → %s", name, safeDisplayText(r.Path))
		if len(r.DesktopFiles) > 0 {
			name += fmt.Sprintf(" (%d acceso(s) de menú)", len(r.DesktopFiles))
		}
	}
	if r.Source == SourceWebApp {
		name += " (lanzador web)"
	}

	if active {
		return tag + " " + selectedStyle.Render(name)
	}
	return tag + " " + name
}

// safeDisplayText prevents metadata and paths from injecting terminal control
// sequences into the TUI.
func safeDisplayText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, value)
}

func keyHint(key, action string) string {
	return keyStyle.Render("["+key+"]") + " " + dimStyle.Render(action)
}

func truncateDisplayText(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes-1]) + "…"
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
func doScanInstalled() tea.Cmd {
	return func() tea.Msg { return catalogDoneMsg{result: ScanInstalled()} }
}

func doSudoPassword(password string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sudo", "-S", "-v")
		cmd.Stdin = strings.NewReader(password + "\n")
		err := cmd.Run()
		// Best-effort: avoid keeping the password string longer than needed.
		password = ""
		if err != nil {
			return sudoAuthDoneMsg{err: err}
		}
		return sudoAuthDoneMsg{err: nil}
	}
}

func doUninstall(pkg PackageResult, mode string, dataDirs, desktopFiles []string) tea.Cmd {
	return func() tea.Msg {
		err := UninstallWithDataAndDesktopFiles(pkg, mode, dataDirs, desktopFiles)
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

func doLoadHistory() tea.Cmd {
	return func() tea.Msg {
		history, err := LoadHistory()
		return historyMsg{history: history, err: err}
	}
}

func doLoadConfirmDeps(pkg PackageResult, mode string) tea.Cmd {
	return func() tea.Msg {
		info := GetPackageInfo(pkg)
		msg := confirmDepsMsg{
			deps:      info.Dependents,
			depsKnown: info.DependentsKnown,
		}
		if pkg.Source == SourceAPT || pkg.Source == SourcePacman {
			msg.depsErr = info.DependentsError
		}
		if mode == modeComplete {
			switch pkg.Source {
			case SourceAPT:
				msg.removals, msg.removalErr = previewAPTRemovals(pkg.Name)
			case SourcePacman:
				msg.removals, msg.removalErr = previewPacmanRemovals(pkg.Name)
			}
		}
		if mode == modeComplete {
			msg.dataDirs = candidateCompleteDataDirs(pkg)
			msg.desktopFiles = candidateCompleteDesktopFiles(pkg)
		}
		return msg
	}
}
