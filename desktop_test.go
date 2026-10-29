package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopExecProgram(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{name: "plain executable and arguments", line: "/usr/bin/firefox --new-window", want: "/usr/bin/firefox", ok: true},
		{name: "quoted executable path", line: `"/home/test/My Browser/browser" --profile default`, want: "/home/test/My Browser/browser", ok: true},
		{name: "escaped whitespace", line: `/home/test/My\ Browser/browser --profile default`, want: "/home/test/My Browser/browser", ok: true},
		{name: "leading whitespace", line: "  /usr/bin/firefox\t--new-window", want: "/usr/bin/firefox", ok: true},
		{name: "empty", line: "", ok: false},
		{name: "empty quoted program", line: `"" --flag`, ok: false},
		{name: "unclosed quote", line: `"/usr/bin/firefox`, ok: false},
		{name: "trailing escape", line: "/usr/bin/firefox\\", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := desktopExecProgram(tt.line)
			if got != tt.want || ok != tt.ok {
				t.Errorf("desktopExecProgram(%q) = (%q, %v); want (%q, %v)", tt.line, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDesktopEntryWebAppAcceptsOnlyVisibleOmarchyLaunchers(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "valid launcher",
			data: "[Desktop Entry]\nType=Application\nName=Discord\nExec=omarchy-launch-webapp https://discord.com\n",
			want: true,
		},
		{
			name: "hidden entry",
			data: "[Desktop Entry]\nType=Application\nName=Discord\nExec=omarchy-launch-webapp https://discord.com\nHidden=true\n",
		},
		{
			name: "no display entry",
			data: "[Desktop Entry]\nType=Application\nName=Discord\nExec=omarchy-launch-webapp https://discord.com\nNoDisplay=true\n",
		},
		{
			name: "non Omarchy launcher",
			data: "[Desktop Entry]\nType=Application\nName=Discord\nExec=firefox https://discord.com\n",
		},
		{
			name: "non application",
			data: "[Desktop Entry]\nType=Link\nName=Discord\nExec=omarchy-launch-webapp https://discord.com\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "entry.desktop")
			if err := os.WriteFile(path, []byte(tt.data), 0o600); err != nil {
				t.Fatal(err)
			}
			name, _, ok := desktopEntryWebApp(path)
			if ok != tt.want {
				t.Fatalf("desktopEntryWebApp() accepted=%v; want %v", ok, tt.want)
			}
			if ok && name != "Discord" {
				t.Errorf("desktopEntryWebApp() name = %q; want Discord", name)
			}
		})
	}
}

func TestValidateWebAppDesktopEntryRestrictsPathAndContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataHome := filepath.Join(home, ".local", "share")
	t.Setenv("XDG_DATA_HOME", dataHome)
	applications := filepath.Join(dataHome, "applications")
	if err := os.MkdirAll(applications, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(applications, "discord.desktop")
	content := "[Desktop Entry]\nType=Application\nName=Discord\nExec=omarchy-launch-webapp https://discord.com\n"
	if err := os.WriteFile(entry, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := validateWebAppDesktopEntry(entry, "Discord", "omarchy-launch-webapp https://discord.com"); err != nil {
		t.Fatalf("valid launcher rejected: %v", err)
	}
	if err := validateWebAppDesktopEntry(entry, "Discord", "omarchy-launch-webapp https://example.com"); err == nil {
		t.Fatal("launcher with mismatched Exec command was accepted")
	}
	if err := validateWebAppDesktopEntry(filepath.Join(home, "outside.desktop"), "Discord", "omarchy-launch-webapp https://discord.com"); err == nil {
		t.Fatal("desktop file outside the applications directory was accepted")
	}
}

func TestValidateAppImageDesktopFileRequiresExactTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dataHome := filepath.Join(home, ".local", "share")
	t.Setenv("XDG_DATA_HOME", dataHome)
	applications := filepath.Join(dataHome, "applications")
	if err := os.MkdirAll(applications, 0o700); err != nil {
		t.Fatal(err)
	}
	appImage := filepath.Join(home, "Applications", "Example.AppImage")
	entry := filepath.Join(applications, "example.desktop")
	if err := os.WriteFile(entry, []byte("[Desktop Entry]\nType=Application\nExec=\""+appImage+"\" %U\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateAppImageDesktopFile(entry, appImage); err != nil {
		t.Fatalf("matching launcher rejected: %v", err)
	}
	if err := validateAppImageDesktopFile(entry, filepath.Join(home, "Applications", "Other.AppImage")); err == nil {
		t.Fatal("launcher pointing at a different AppImage was accepted")
	}
}
