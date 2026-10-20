package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Orvaen/terminal/internal/platform"
)

// pathSetup : un PATH de test avec deux make, et un fichier de réglages isolé.
func pathSetup(t *testing.T) (old, nouveau string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("scripts sh")
	}
	old, nouveau = t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(old, "make"), []byte("#!/bin/sh\necho GNU Make 3.82\n"), 0o755)
	os.WriteFile(filepath.Join(nouveau, "make"), []byte("#!/bin/sh\necho GNU Make 4.4.1\n"), 0o755)
	os.Symlink(filepath.Join(nouveau, "make"), filepath.Join(old, "make-lien"))
	t.Setenv("PATH", strings.Join([]string{old, "/introuvable", nouveau, old}, string(os.PathListSeparator)))
	cfg := filepath.Join(t.TempDir(), "terminal-config.json")
	oldCfg, oldReg, oldVer := configPath, registryPath, versionLine
	configPath = func() string { return cfg }
	registryPath = func(scope string) []platform.PathEntry {
		if scope == "system" {
			return []platform.PathEntry{{Raw: old, Dir: old}}
		}
		return nil
	}
	versionLine = func(p string) string {
		if strings.HasPrefix(p, old) {
			return "GNU Make 3.82"
		}
		return "GNU Make 4.4.1"
	}
	t.Cleanup(func() { configPath, registryPath, versionLine = oldCfg, oldReg, oldVer })
	return old, nouveau
}

func TestPathWhoAndMonter(t *testing.T) {
	old, nouveau := pathSetup(t)
	s, out, errOut, _ := testShell(t)

	s.RunLine("path qui make")
	if got := out.String(); !strings.Contains(got, "→  1. "+filepath.Join(old, "make")) || !strings.Contains(got, "2. "+filepath.Join(nouveau, "make")) || !strings.Contains(got, "path monter make 2") {
		t.Fatalf("path qui : %q", got)
	}
	out.Reset()
	if code := s.RunLine("path monter make 2"); code != 0 {
		t.Fatalf("code %d : %s", code, errOut)
	}
	if got := matches("make"); got[0] != filepath.Join(nouveau, "make") {
		t.Fatalf("après monter : %v", got)
	}
	// Gardé pour les prochaines fenêtres : un nouveau PATH dans le désordre est rangé au démarrage.
	t.Setenv("PATH", strings.Join([]string{old, nouveau}, string(os.PathListSeparator)))
	applyPathPrefs()
	if first := pathDirs()[0]; first != nouveau {
		t.Fatalf("ordre non réappliqué : %v", pathDirs())
	}

	s.RunLine("path masquer " + old)
	if containsPath(pathDirs(), old) {
		t.Fatalf("dossier masqué encore présent : %v", pathDirs())
	}
	p := loadPathPrefs()
	if !reflect.DeepEqual(p.First, []string{nouveau}) || !reflect.DeepEqual(p.Hidden, []string{old}) {
		t.Fatalf("réglages : %+v", p)
	}
	s.RunLine("path reset")
	if p := loadPathPrefs(); len(p.First)+len(p.Hidden) != 0 {
		t.Fatalf("reset : %+v", p)
	}
}

func TestPathListMarksDeadAndDuplicates(t *testing.T) {
	old, _ := pathSetup(t)
	s, out, _, _ := testShell(t)
	s.RunLine("path")
	got := out.String()
	for _, want := range []string{"n'existe pas", "doublon du n°1", "Windows, système", "path nettoyer"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q manque :\n%s", want, got)
		}
	}
	_ = old
}

func TestMatchesSkipsSameFile(t *testing.T) {
	old, nouveau := pathSetup(t)
	os.Symlink(filepath.Join(nouveau, "make"), filepath.Join(old, "make2"))
	t.Setenv("PATH", old+string(os.PathListSeparator)+nouveau)
	os.WriteFile(filepath.Join(old, "outil"), []byte("#!/bin/sh\n"), 0o755)
	os.Link(filepath.Join(old, "outil"), filepath.Join(nouveau, "outil"))
	if got := matches("outil"); len(got) != 1 {
		t.Fatalf("le même fichier ne doit compter qu'une fois : %v", got)
	}
}

func TestTargetForms(t *testing.T) {
	old, nouveau := pathSetup(t)
	if d, err := target([]string{"1"}); err != nil || d != old {
		t.Fatalf("n° : %q %v", d, err)
	}
	if d, err := target([]string{"make", "2"}); err != nil || d != nouveau {
		t.Fatalf("commande + n° : %q %v", d, err)
	}
	if _, err := target([]string{"99"}); err == nil {
		t.Fatal("n° hors liste")
	}
	if _, err := target([]string{"make", "5"}); err == nil {
		t.Fatal("version inexistante")
	}
	if _, err := target([]string{"/pas/un/dossier"}); err == nil {
		t.Fatal("dossier inexistant")
	}
}

func TestCleanEntriesKeepsVariables(t *testing.T) {
	exists := func(d string) bool { return !strings.Contains(d, "Disparu") }
	system := []platform.PathEntry{{Raw: `%SystemRoot%\system32`, Dir: `C:\Windows\system32`}}
	user := []platform.PathEntry{
		{Raw: `%USERPROFILE%\bin`, Dir: `C:\Users\r\bin`},
		{Raw: `C:\Users\r\bin\`, Dir: `C:\Users\r\bin\`},         // doublon (\ final)
		{Raw: `C:\WINDOWS\System32`, Dir: `C:\WINDOWS\System32`}, // déjà dans le PATH système
		{Raw: `C:\Disparu`, Dir: `C:\Disparu`},                   // n'existe plus
		{Raw: `%LOCALAPPDATA%\x`, Dir: `C:\Users\r\AppData\Local\x`},
	}
	keep, removed := cleanEntries(user, system, exists)
	if !reflect.DeepEqual(keep, []string{`%USERPROFILE%\bin`, `%LOCALAPPDATA%\x`}) {
		t.Fatalf("gardés : %v", keep)
	}
	if len(removed) != 3 {
		t.Fatalf("retirés : %v", removed)
	}
}
