package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
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

// Key bindings
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Esc     key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Help    key.Binding
	Filter1 key.Binding
	Filter2 key.Binding
	Filter3 key.Binding
	Filter4 key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Esc, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Esc},
		{k.Help, k.Filter1, k.Filter2, k.Filter3, k.Filter4},
	}
}

var keys = keyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "subir")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "bajar")),
	Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "seleccionar")),
	Esc:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "atrás/salir")),
	Confirm: key.NewBinding(key.WithKeys("y", "s"), key.WithHelp("y/s", "confirmar")),
	Cancel:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "cancelar")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ayuda")),
	Filter1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "APT")),
	Filter2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Flatpak")),
	Filter3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Snap")),
	Filter4: key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "AppImage")),
}

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
	state      int
	textInput  textinput.Model
	spinner    spinner.Model
	viewport   viewport.Model
	help       help.Model
	searchTerm string
	results    []PackageResult
	allResults []PackageResult
	cursor     int
	selected   PackageResult
	modeCursor int
	mode       string
	err        error
	quitting   bool
	scanErrors []ScanError
	filter     string
	width      int
	height     int
	pkgInfo    *PackageInfo
	dryResult  *DryRunResult
}

// InitModel creates a new model, optionally with a pre-filled search term.
func InitModel(term string) Model {
	ti := textinput.New()
	ti.Placeholder = "Ej: firefox, vlc, gimp..."
	ti.Width = 60
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	state := stateInput
	if term != "" {
		state = stateScanning
	}

	return Model{
		state:      state,
		textInput:  ti,
		spinner:    sp,
		searchTerm: term,
		help:       help.New(),
		viewport:   viewport.New(80, 20),
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
			m.err = fmt.Errorf("error de autenticación: %w", msg.err)
			m.state = stateDone
			return m, nil
		}
		m.state = stateExecuting
		return m, tea.Batch(m.spinner.Tick, doUninstall(m.selected, m.mode))
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
			m.filter = SourceAPT
			m.applyFilter()
		case "2":
			m.filter = SourceFlatpak
			m.applyFilter()
		case "3":
			m.filter = SourceSnap
			m.applyFilter()
		case "4":
			m.filter = SourceAppImage
			m.applyFilter()
		case "0":
			m.filter = ""
			m.results = m.allResults
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
			m.state = stateSudoAuth
			return m, tea.Batch(m.spinner.Tick, doSudoAuth())
		case "n", "esc":
			m.state = stateModeSelect
		}
	}
	return m, nil
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
	if _, ok := msg.(tea.KeyMsg); ok {
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return dimStyle.Render("Operación cancelada.") + "\n"
	}

	if m.state == stateHelp {
		return m.viewHelp()
	}

	switch m.state {
	case stateInput:
		return m.viewInput()
	case stateScanning:
		return m.viewScanning()
	case stateResults:
		return m.viewResults()
	case stateModeSelect:
		return m.viewModeSelect()
	case stateConfirm:
		return m.viewConfirm()
	case stateSudoAuth:
		return m.viewSudoAuth()
	case stateExecuting:
		return m.viewExecuting()
	case stateDone:
		return m.viewDone()
	case statePackageInfo:
		return m.viewPackageInfo()
	case stateDryRun:
		return m.viewDryRun()
	}
	return ""
}

func (m Model) viewInput() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Desinstalador interactivo"))
	b.WriteString("\n\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("enter: buscar • esc: salir • ?: ayuda"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("────────────────────────────────────────────────"))
	return b.String()
}

func (m Model) viewScanning() string {
	return fmt.Sprintf("%s\n\n%s Buscando \"%s\" en el sistema...\n",
		renderBanner(),
		m.spinner.View(),
		m.searchTerm,
	)
}

func (m Model) viewResults() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%d resultados para \"%s\"", len(m.results), m.searchTerm)))
	if m.filter != "" {
		b.WriteString(" ")
		b.WriteString(sourceTag(m.filter).Render("[Filtro: " + m.filter + "]"))
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
	b.WriteString(dimStyle.Render("↑/↓: navegar • enter: seleccionar • esc: volver • 1-4: filtrar • ?: ayuda"))
	return b.String()
}

func (m Model) viewModeSelect() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Programa seleccionado: %s\n\n", selectedStyle.Render(m.selected.Name)))
	b.WriteString(subtitleStyle.Render("¿Cómo deseas proceder?"))
	b.WriteString("\n\n")

	options := []string{
		"Desinstalación Normal (conserva tus configuraciones)",
		"Desinstalación Completa (borra TODO, sin dejar rastro)",
		"Volver a resultados",
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
	b.WriteString(dimStyle.Render("↑/↓: navegar • enter: seleccionar • esc: volver"))
	return b.String()
}

func (m Model) viewConfirm() string {
	modeText := "normal"
	if m.mode == modeComplete {
		modeText = "completa (purga)"
	}
	alert := fmt.Sprintf("¿Estás seguro de eliminar \"%s\" [%s] en modo %s?",
		m.selected.Name, m.selected.Source, modeText)
	return fmt.Sprintf("%s\n\n%s\n\n%s\n",
		renderBanner(),
		warningStyle.Render(alert),
		dimStyle.Render("y/s: confirmar • n/esc: cancelar"),
	)
}

func (m Model) viewSudoAuth() string {
	return fmt.Sprintf("%s\n\n%s Solicitando permisos de administrador...\n",
		renderBanner(),
		m.spinner.View(),
	)
}

func (m Model) viewExecuting() string {
	return fmt.Sprintf("%s\n\n%s Desinstalando %s...\n",
		renderBanner(),
		m.spinner.View(),
		m.selected.Name,
	)
}

func (m Model) viewDone() string {
	if len(m.results) == 0 && m.err == nil {
		return fmt.Sprintf("%s\n\n%s\n",
			renderBanner(),
			dimStyle.Render("No se encontró nada que coincida con \""+m.searchTerm+"\"."),
		)
	}
	if m.err != nil {
		return fmt.Sprintf("%s\n\n%s\n%s\n\n%s\n",
			renderBanner(),
			errorStyle.Render("✗ Error durante la desinstalación:"),
			errorStyle.Render(m.err.Error()),
			dimStyle.Render("Presiona cualquier tecla para salir."),
		)
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s\n",
		renderBanner(),
		successStyle.Render("✓ ¡Listo! \""+m.selected.Name+"\" ha sido eliminado con éxito."),
		dimStyle.Render("Presiona cualquier tecla para salir."),
	)
}

func (m Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Ayuda"))
	b.WriteString("\n\n")
	b.WriteString(subtitleStyle.Render("Navegación:"))
	b.WriteString("\n")
	b.WriteString("  ↑/k  Subir en la lista\n")
	b.WriteString("  ↓/j  Bajar en la lista\n")
	b.WriteString("  enter  Seleccionar/Confirmar\n")
	b.WriteString("  esc  Volver atrás / Salir\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Acciones (en resultados):"))
	b.WriteString("\n")
	b.WriteString("  i  Ver información del paquete\n")
	b.WriteString("  d  Dry-run (simular desinstalación)\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Filtros (en resultados):"))
	b.WriteString("\n")
	b.WriteString("  1  Mostrar solo APT\n")
	b.WriteString("  2  Mostrar solo Flatpak\n")
	b.WriteString("  3  Mostrar solo Snap\n")
	b.WriteString("  4  Mostrar solo AppImage\n")
	b.WriteString("  0  Sin filtro\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Modos de desinstalación:"))
	b.WriteString("\n")
	b.WriteString("  Normal:   Elimina el programa, conserva configuraciones\n")
	b.WriteString("  Completa: Elimina el programa y TODOS sus datos\n")
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Presiona esc o ? para volver"))
	return b.String()
}

func (m Model) viewPackageInfo() string {
	if m.pkgInfo == nil {
		return fmt.Sprintf("%s\n\n%s Cargando información...\n",
			renderBanner(),
			m.spinner.View(),
		)
	}
	info := m.pkgInfo
	var b strings.Builder
	b.WriteString(renderBannerSuffix("Información del paquete"))
	b.WriteString("\n\n")
	b.WriteString("  Nombre:      " + selectedStyle.Render(info.Name) + "\n")
	if info.Version != "" {
		b.WriteString("  Versión:     " + info.Version + "\n")
	}
	if info.InstalledSize != "" {
		b.WriteString("  Tamaño:      " + info.InstalledSize + "\n")
	}
	if info.Description != "" {
		b.WriteString("  Descripción: " + info.Description + "\n")
	}
	if len(info.Dependents) > 0 {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render("  ⚠ Dependencias inversas ("+fmt.Sprintf("%d", len(info.Dependents))+"):") + "\n")
		for _, dep := range info.Dependents {
			b.WriteString("    - " + dep + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Presiona esc, enter o i para volver"))
	return b.String()
}

func (m Model) viewDryRun() string {
	if m.dryResult == nil {
		return fmt.Sprintf("%s\n\n%s Simulando desinstalación...\n",
			renderBanner(),
			m.spinner.View(),
		)
	}
	result := m.dryResult

	var b strings.Builder
	b.WriteString(renderBannerSuffix("Dry Run (Simulación)"))
	b.WriteString("\n\n")
	b.WriteString("  Paquete: " + selectedStyle.Render(m.selected.Name) + " [" + m.selected.Source + "]\n")
	modeText := "Normal"
	if m.mode == modeComplete {
		modeText = "Completa (purga)"
	}
	b.WriteString("  Modo:    " + modeText + "\n")
	b.WriteString("  Espacio estimado a liberar: " + successStyle.Render(FormatBytes(result.EstimateBytes)) + "\n")
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Comandos que se ejecutarían:"))
	b.WriteString("\n")
	for _, cmd := range result.Commands {
		b.WriteString("  $ " + cmd + "\n")
	}
	if len(result.DirsToRemove) > 0 {
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Directorios que se eliminarían:"))
		b.WriteString("\n")
		for _, dir := range result.DirsToRemove {
			b.WriteString("  - " + dir + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Presiona esc, enter o d para volver"))
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

// Commands
func doScan(term string) tea.Cmd {
	return func() tea.Msg {
		result := ScanAll(term)
		return scanDoneMsg{result: result}
	}
}

func doSudoAuth() tea.Cmd {
	return tea.ExecProcess(
		exec.Command("sudo", "-v", "--prompt=Contraseña de sudo: "),
		func(err error) tea.Msg {
			if err != nil {
				return sudoAuthDoneMsg{err: fmt.Errorf("autenticación fallida: %w", err)}
			}
			return sudoAuthDoneMsg{err: nil}
		},
	)
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
