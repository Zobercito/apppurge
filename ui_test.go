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
