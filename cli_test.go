package main

import "testing"

func TestParseCLIOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    cliOptions
		wantErr bool
	}{
		{name: "interactive defaults", want: cliOptions{}},
		{name: "joins search terms", args: []string{"Discord", "Web App"}, want: cliOptions{searchTerm: "Discord Web App"}},
		{name: "source and search", args: []string{"--source", "pacman", "firefox"}, want: cliOptions{searchTerm: "firefox", source: SourcePacman}},
		{name: "equals source spelling", args: []string{"--source=web-app", "Discord"}, want: cliOptions{searchTerm: "Discord", source: SourceWebApp}},
		{name: "complete dry run", args: []string{"--complete", "--dry-run", "chromium"}, want: cliOptions{searchTerm: "chromium", dryRun: true, complete: true}},
		{name: "help", args: []string{"--help"}, want: cliOptions{help: true}},
		{name: "version", args: []string{"--version"}, want: cliOptions{version: true}},
		{name: "missing source", args: []string{"--source"}, wantErr: true},
		{name: "unknown source", args: []string{"--source=dnf", "browser"}, wantErr: true},
		{name: "unknown option", args: []string{"--remove-everything"}, wantErr: true},
		{name: "complete without dry run", args: []string{"--complete", "browser"}, wantErr: true},
		{name: "dry run without query", args: []string{"--dry-run"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCLIOptions(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCLIOptions(%q) error = %v; wantErr %v", tt.args, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseCLIOptions(%q) = %+v; want %+v", tt.args, got, tt.want)
			}
		})
	}
}
