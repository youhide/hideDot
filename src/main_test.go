package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExpandPath(t *testing.T) {
	home := "/home/user"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde only", "~", home},
		{"tilde slash", "~/.zshrc", filepath.Join(home, ".zshrc")},
		{"absolute", "/etc/hosts", "/etc/hosts"},
		{"relative", "foo/bar", "foo/bar"},
		{"tilde no slash", "~foo", "~foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandPath(tt.in, home); got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpandSourcePath(t *testing.T) {
	// Use filepath.Abs so the "absolute" cases are truly absolute on every OS
	// (a leading "/" is not absolute on Windows, which lacks a drive letter).
	home, err := filepath.Abs(filepath.Join("home", "user"))
	if err != nil {
		t.Fatal(err)
	}
	execDir, err := filepath.Abs(filepath.Join("repo", "dotfiles"))
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join("opt", "x"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"absolute", abs, abs},
		{"tilde", "~/src", filepath.Join(home, "src")},
		{"relative to execDir", filepath.Join("zsh", "zshrc"), filepath.Join(execDir, "zsh", "zshrc")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandSourcePath(tt.in, home, execDir); got != tt.want {
				t.Errorf("expandSourcePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetBackupPath(t *testing.T) {
	backupDir := t.TempDir()
	app := &App{backupDir: backupDir}
	p1 := app.getBackupPath(filepath.Join("home", "user", ".zshrc"))
	p2 := app.getBackupPath(filepath.Join("home", "user", ".zshrc"))
	p3 := app.getBackupPath(filepath.Join("home", "user", ".vimrc"))

	if p1 != p2 {
		t.Errorf("getBackupPath not deterministic: %q vs %q", p1, p2)
	}
	if p1 == p3 {
		t.Errorf("getBackupPath collision for different inputs: %q", p1)
	}
	if filepath.Dir(p1) != backupDir {
		t.Errorf("backup not under backupDir: %q", p1)
	}
	if !strings.HasPrefix(filepath.Base(p1), ".zshrc_") {
		t.Errorf("backup base should keep original name: %q", filepath.Base(p1))
	}
}

func TestLinkStatusString(t *testing.T) {
	tests := map[LinkStatus]string{
		StatusOK:         "OK",
		StatusMissing:    "MISSING",
		StatusBroken:     "BROKEN",
		StatusMismatch:   "MISMATCH",
		StatusNotSymlink: "NOT_SYMLINK",
		LinkStatus(99):   "UNKNOWN",
	}
	for status, want := range tests {
		if got := status.String(); got != want {
			t.Errorf("LinkStatus(%d).String() = %q, want %q", status, got, want)
		}
	}
}

func TestShellCommandUnmarshalYAML(t *testing.T) {
	t.Run("array form", func(t *testing.T) {
		var cmd ShellCommand
		if err := yaml.Unmarshal([]byte(`["touch ~/.x", "make x"]`), &cmd); err != nil {
			t.Fatal(err)
		}
		if cmd.Command != "touch ~/.x" || cmd.Description != "make x" || cmd.Stdin != "" {
			t.Errorf("array form parsed wrong: %+v", cmd)
		}
	})

	t.Run("map form with stdin", func(t *testing.T) {
		var cmd ShellCommand
		src := "command: cat > ~/.c\ndescription: write\nstdin: hello\n"
		if err := yaml.Unmarshal([]byte(src), &cmd); err != nil {
			t.Fatal(err)
		}
		if cmd.Command != "cat > ~/.c" || cmd.Description != "write" || cmd.Stdin != "hello" {
			t.Errorf("map form parsed wrong: %+v", cmd)
		}
	})
}

func TestExpandTemplates(t *testing.T) {
	app := &App{tmplData: TemplateData{
		Hostname: "myhost",
		Username: "bob",
		OS:       "darwin",
		Arch:     "arm64",
	}}
	out, err := app.expandTemplates("host={{ .Hostname }} os={{ .OS }}/{{ .Arch }} user={{ .Username }}")
	if err != nil {
		t.Fatal(err)
	}
	want := "host=myhost os=darwin/arm64 user=bob"
	if out != want {
		t.Errorf("expandTemplates = %q, want %q", out, want)
	}

	// Invalid template returns original content unchanged.
	orig := "value={{ .Missing "
	if got, _ := app.expandTemplates(orig); got != orig {
		t.Errorf("invalid template should return original, got %q", got)
	}
}

func TestCheckLinkStatus(t *testing.T) {
	dir := t.TempDir()
	app := &App{homeDir: dir, execDir: dir}

	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	// OK: correct symlink
	okLink := filepath.Join(dir, "ok")
	if err := os.Symlink(source, okLink); err != nil {
		t.Fatal(err)
	}
	if got := app.checkLinkStatus(okLink, source).Status; got != StatusOK {
		t.Errorf("expected OK, got %v", got)
	}

	// MISSING: nothing there
	if got := app.checkLinkStatus(filepath.Join(dir, "nope"), source).Status; got != StatusMissing {
		t.Errorf("expected MISSING, got %v", got)
	}

	// NOT_SYMLINK: a regular file
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := app.checkLinkStatus(regular, source).Status; got != StatusNotSymlink {
		t.Errorf("expected NOT_SYMLINK, got %v", got)
	}

	// MISMATCH: symlink to a different target
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(other, []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	mismatch := filepath.Join(dir, "mismatch")
	if err := os.Symlink(other, mismatch); err != nil {
		t.Fatal(err)
	}
	if got := app.checkLinkStatus(mismatch, source).Status; got != StatusMismatch {
		t.Errorf("expected MISMATCH, got %v", got)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("content"), 0640); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "nested", "dst")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Errorf("copied content = %q", got)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	// Unix permission bits don't map onto Windows, where Go reports 0666/0444.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Errorf("copied mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestCopyDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a"), []byte("A"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b"), []byte("B"), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{"a": "A", filepath.Join("sub", "b"): "B"} {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestBuildShellCmd(t *testing.T) {
	cmd := buildShellCmd("echo hi")
	var wantArgs []string
	if runtime.GOOS == "windows" {
		wantArgs = []string{"cmd", "/c", "echo hi"}
	} else {
		wantArgs = []string{"bash", "-c", "echo hi"}
	}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", cmd.Args, wantArgs)
	}
	for i := range wantArgs {
		if cmd.Args[i] != wantArgs[i] {
			t.Errorf("arg[%d] = %q, want %q", i, cmd.Args[i], wantArgs[i])
		}
	}
}
