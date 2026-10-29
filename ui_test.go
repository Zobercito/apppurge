package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestViewConfirmShowsRemovalAndDataPreviews(t *testing.T) {
	m := Model{
		state:            stateConfirm,
		confirmDepsReady: true,
		selected:         PackageResult{Name: "app", Source: SourcePacman},
		mode:             modeComplete,
		confirmRemovals:  []string{"app", "shared-dep"},
		confirmDataDirs:  []string{"/home/test/.config/app"},
	}
	view := m.viewConfirm()
	for _, want := range []string{
		"También se quitarán 2 paquete(s)",
		"shared-dep",
		"Se eliminarán estas carpetas de datos:",
		"/home/test/.config/app",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("viewConfirm() does not contain %q", want)
		}
	}
}

func TestUpdateConfirmBlocksPacmanWithoutRemovalPreview(t *testing.T) {
	m := Model{
		state:             stateConfirm,
		confirmDepsReady:  true,
		confirmRemovalErr: "pacman unavailable",
		selected:          PackageResult{Name: "app", Source: SourcePacman},
		mode:              modeComplete,
	}
	updated, _ := m.updateConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if updated.(Model).state != stateConfirm {
		t.Fatal("confirmation advanced despite a failed Pacman removal preview")
	}
}

func TestViewConfirmReportsUnavailablePacmanPreview(t *testing.T) {
	m := Model{
		confirmDepsReady:  true,
		selected:          PackageResult{Name: "app", Source: SourcePacman},
		mode:              modeComplete,
		confirmRemovalErr: "pacman unavailable",
	}
	if view := m.viewConfirm(); !strings.Contains(view, "No se pudo previsualizar lo que se quitará: pacman unavailable") {
		t.Errorf("viewConfirm() does not explain the missing preview: %s", view)
	}
}

func TestConfirmBlockedPreviewOffersRetry(t *testing.T) {
	m := Model{
		state:             stateConfirm,
		confirmDepsReady:  true,
		confirmRemovalErr: "pacman unavailable",
		selected:          PackageResult{Name: "app", Source: SourcePacman},
		mode:              modeComplete,
	}
	if view := m.viewConfirm(); !strings.Contains(view, "reintentar comprobaciones") {
		t.Errorf("blocked confirmation has no retry hint: %s", view)
	}
	updated, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil || updated.(Model).confirmDepsReady {
		t.Fatal("retry did not restart the dependency preview")
	}
}

func TestSearchFromCatalogMatchesWebAppExecID(t *testing.T) {
	m := Model{
		catalog: []PackageResult{{Name: "Discord", Source: SourceWebApp, ID: "omarchy-launch-webapp https://discord.com/channels/@me"}},
	}
	m.searchFromCatalog("discord.com")
	if len(m.results) != 1 || m.results[0].Name != "Discord" {
		t.Fatalf("searchFromCatalog() results = %#v, want Discord web app", m.results)
	}
}

func TestRefreshSuggestionsClearsWhenInputIsBlank(t *testing.T) {
	input := textinput.New()
	m := Model{
		textInput:        input,
		catalogLoaded:    true,
		catalog:          []PackageResult{{Name: "Chromium", Source: SourcePacman}},
		suggestions:      []PackageResult{{Name: "Stale", Source: SourcePacman}},
		suggestionCursor: 1,
	}
	m.refreshSuggestions()
	if len(m.suggestions) != 0 || m.suggestionCursor != 0 {
		t.Fatalf("blank input left suggestions or cursor: %v, %d", m.suggestions, m.suggestionCursor)
	}
	if view := m.viewInput(); strings.Contains(view, "Sugerencias") {
		t.Errorf("blank input still renders suggestions: %s", view)
	}
}

func TestUpdateInputCanRefreshCatalog(t *testing.T) {
	m := Model{
		state:         stateInput,
		textInput:     textinput.New(),
		catalogLoaded: true,
		catalog:       []PackageResult{{Name: "stale", Source: SourcePacman}},
	}
	updated, cmd := m.updateInput(tea.KeyMsg{Type: tea.KeyF5})
	got := updated.(Model)
	if cmd == nil || !got.catalogLoading || got.catalogLoaded || len(got.catalog) != 0 {
		t.Fatalf("F5 did not start a clean catalog refresh: model=%+v cmd=%v", got, cmd)
	}
}

func TestResultsNavigationAndSourceFilters(t *testing.T) {
	all := []PackageResult{
		{Name: "apt-app", Source: SourceAPT},
		{Name: "flatpak-app", Source: SourceFlatpak},
		{Name: "pacman-app", Source: SourcePacman},
	}

	tests := []struct {
		name       string
		key        tea.KeyMsg
		wantCursor int
		wantFilter string
		wantCount  int
	}{
		{name: "down moves cursor", key: tea.KeyMsg{Type: tea.KeyDown}, wantCursor: 1, wantCount: 3},
		{name: "up stays at first result", key: tea.KeyMsg{Type: tea.KeyUp}, wantCursor: 0, wantCount: 3},
		{name: "filter pacman", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}}, wantFilter: SourcePacman, wantCount: 1},
		{name: "all sources resets filter", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, wantCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{state: stateResults, allResults: all, results: all, cursor: 0}
			if tt.name == "up stays at first result" {
				m.cursor = 0
			}
			updated, _ := m.updateResults(tt.key)
			got := updated.(Model)
			if got.cursor != tt.wantCursor {
				t.Errorf("cursor = %d; want %d", got.cursor, tt.wantCursor)
			}
			if got.filter != tt.wantFilter {
				t.Errorf("filter = %q; want %q", got.filter, tt.wantFilter)
			}
			if len(got.results) != tt.wantCount {
				t.Errorf("results count = %d; want %d", len(got.results), tt.wantCount)
			}
		})
	}
}

func TestResultPageNavigationClampsCursor(t *testing.T) {
	results := make([]PackageResult, 30)
	m := Model{state: stateResults, results: results, height: 24}
	pageSize := m.resultsPageSize()

	updated, _ := m.updateResults(tea.KeyMsg{Type: tea.KeyPgDown})
	got := updated.(Model)
	if got.cursor != pageSize {
		t.Fatalf("cursor after PgDn = %d; want %d", got.cursor, pageSize)
	}

	updated, _ = got.updateResults(tea.KeyMsg{Type: tea.KeyPgDown})
	got = updated.(Model)
	updated, _ = got.updateResults(tea.KeyMsg{Type: tea.KeyPgDown})
	got = updated.(Model)
	if got.cursor != len(results)-1 {
		t.Fatalf("cursor after PgDn at end = %d; want %d", got.cursor, len(results)-1)
	}

	updated, _ = got.updateResults(tea.KeyMsg{Type: tea.KeyPgUp})
	got = updated.(Model)
	if got.cursor != len(results)-1-pageSize {
		t.Fatalf("cursor after PgUp = %d; want %d", got.cursor, len(results)-1-pageSize)
	}
}

func TestHelpReturnsToTheScreenItWasOpenedFrom(t *testing.T) {
	m := Model{state: stateResults}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	got := updated.(Model)
	if got.state != stateHelp || got.helpReturnState != stateResults {
		t.Fatalf("opening help produced state=%d return=%d", got.state, got.helpReturnState)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if got.state != stateResults {
		t.Fatalf("closing help returned to state %d; want results", got.state)
	}
}

func TestEscapeFromResultsResetsSearchAndFilter(t *testing.T) {
	input := textinput.New()
	input.SetValue("pacman")
	m := Model{
		state:         stateResults,
		textInput:     input,
		searchTerm:    "pacman",
		filter:        SourcePacman,
		results:       []PackageResult{{Name: "pkg", Source: SourcePacman}},
		allResults:    []PackageResult{{Name: "pkg", Source: SourcePacman}},
		suggestions:   []PackageResult{{Name: "pkg", Source: SourcePacman}},
		catalogLoaded: true,
		truncated:     true,
	}
	updated, _ := m.updateResults(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if got.state != stateInput || got.searchTerm != "" || got.textInput.Value() != "" || got.filter != "" {
		t.Fatalf("Esc did not reset search flow: state=%d term=%q input=%q filter=%q", got.state, got.searchTerm, got.textInput.Value(), got.filter)
	}
	if got.truncated || len(got.suggestions) != 0 {
		t.Fatalf("Esc left stale results state: truncated=%v suggestions=%v", got.truncated, got.suggestions)
	}
}

func TestViewConfirmShowsReverseDependencies(t *testing.T) {
	m := Model{
		confirmDepsReady: true,
		confirmDeps:      []string{"desktop-shell", "audio-client"},
		selected:         PackageResult{Name: "shared-lib", Source: SourcePacman},
		mode:             modeNormal,
	}
	view := m.viewConfirm()
	for _, want := range []string{"dependen de este", "desktop-shell", "audio-client"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirmation is missing reverse-dependency detail %q", want)
		}
	}
}

func TestConfirmBlocksSystemPackageRemovalWhenDependencyCheckFailed(t *testing.T) {
	for _, source := range []string{SourceAPT, SourcePacman} {
		t.Run(source, func(t *testing.T) {
			m := Model{
				state:            stateConfirm,
				confirmDepsReady: true,
				confirmDepsErr:   "package database unavailable",
				selected:         PackageResult{Name: "shared-lib", Source: source},
				mode:             modeNormal,
			}
			updated, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			got := updated.(Model)
			if got.state != stateConfirm || cmd != nil {
				t.Fatalf("confirmation proceeded with unknown dependencies: state=%d cmd=%v", got.state, cmd)
			}
		})
	}
}

func TestConfirmRequestsSudoOnlyAfterExplicitConfirmation(t *testing.T) {
	m := Model{
		state:            stateConfirm,
		confirmDepsReady: true,
		confirmDepsKnown: true,
		selected:         PackageResult{Name: "apt-app", Source: SourceAPT},
		mode:             modeNormal,
		passwordInput:    textinput.New(),
	}
	updated, cmd := m.updateConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got := updated.(Model)
	if got.state != statePassword || cmd == nil {
		t.Fatalf("confirmation did not proceed to password prompt: state=%d cmd=%v", got.state, cmd)
	}
}

func TestVisibleWindowKeepsCursorInsideRenderedPage(t *testing.T) {
	tests := []struct {
		name                    string
		total, cursor, pageSize int
		wantStart, wantEnd      int
	}{
		{name: "empty", total: 0, cursor: 0, pageSize: 5, wantStart: 0, wantEnd: 0},
		{name: "short list", total: 3, cursor: 1, pageSize: 5, wantStart: 0, wantEnd: 3},
		{name: "first page", total: 10, cursor: 0, pageSize: 4, wantStart: 0, wantEnd: 4},
		{name: "middle page", total: 10, cursor: 5, pageSize: 4, wantStart: 3, wantEnd: 7},
		{name: "last page", total: 10, cursor: 9, pageSize: 4, wantStart: 6, wantEnd: 10},
		{name: "nonpositive page size", total: 10, cursor: 5, pageSize: 0, wantStart: 0, wantEnd: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := visibleWindow(tt.total, tt.cursor, tt.pageSize)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Fatalf("visibleWindow() = (%d, %d); want (%d, %d)", start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}
