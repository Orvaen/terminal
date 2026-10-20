package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setPSCommands(t *testing.T, names ...string) {
	t.Helper()
	psCommands.Lock()
	old, oldLoaded := psCommands.names, psCommands.loaded
	m := map[string]bool{}
	for _, n := range names {
		m[strings.ToLower(n)] = true
	}
	psCommands.names, psCommands.loaded = m, true
	psCommands.Unlock()
	t.Cleanup(func() {
		psCommands.Lock()
		psCommands.names, psCommands.loaded = old, oldLoaded
		psCommands.Unlock()
	})
}

func TestNotFoundMessageWithInstallHint(t *testing.T) {
	s, out, errOut, _ := testShell(t)
	setPSCommands(t, "Get-Process", "dir")
	if code := s.RunLine("hurl test.hurl -i"); code != 127 {
		t.Fatalf("code %d, attendu 127", code)
	}
	msg := errOut.String()
	if !strings.Contains(msg, "hurl : commande introuvable") || !strings.Contains(msg, "apt install hurl") {
		t.Fatalf("message : %q", msg)
	}
	if strings.Contains(msg, "CommandNotFoundException") || out.Len() != 0 {
		t.Fatal("pas de message de PowerShell")
	}
}

func TestNotFoundInPipeline(t *testing.T) {
	s, _, errOut, _ := testShell(t)
	setPSCommands(t, "Get-Process")
	if code := s.RunLine("machinintrouvable | sort"); code != 127 || !strings.Contains(errOut.String(), "machinintrouvable : commande introuvable") {
		t.Fatalf("code %d : %q", code, errOut)
	}
}

func TestPowerShellCommandsStillGoToPowerShell(t *testing.T) {
	s, _, errOut, _ := testShell(t)
	setPSCommands(t, "Get-Process")
	s.RunLine("Get-Process")
	if strings.Contains(errOut.String(), "commande introuvable") && strings.Contains(errOut.String(), "apt install") {
		t.Fatalf("une commande PowerShell connue ne doit pas être déclarée introuvable : %q", errOut)
	}
	// Liste pas encore connue : PowerShell décide, comme avant.
	psCommands.Lock()
	psCommands.names = nil
	psCommands.Unlock()
	if _, handled := s.notFound("hurl"); handled {
		t.Fatal("sans la liste de PowerShell, le terminal ne doit pas trancher")
	}
	if _, handled := s.notFound(`.\script.ps1`); handled {
		t.Fatal("un chemin n'est jamais déclaré introuvable par le terminal")
	}
}

func TestCommandInstalledSinceStartIsFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script sh")
	}
	s, out, errOut, _ := testShell(t)
	setPSCommands(t)
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "nouvelleappli"), []byte("#!/bin/sh\necho installée $1\n"), 0o755)

	oldGoos, oldRefresh, oldPath := goos, refreshPath, os.Getenv("PATH")
	t.Cleanup(func() { goos, refreshPath = oldGoos, oldRefresh; os.Setenv("PATH", oldPath) })
	goos = "windows"
	refreshPath = func() bool { os.Setenv("PATH", oldPath+string(os.PathListSeparator)+bin); return true }

	if code := s.RunLine("nouvelleappli ok"); code != 0 || out.String() != "installée ok\n" {
		t.Fatalf("code %d, sortie %q, erreurs %q", code, out, errOut)
	}
}

func TestPSCommandsCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "commandes-powershell.txt")
	oldFile, oldList := psCacheFile, listPSCommands
	t.Cleanup(func() { psCacheFile, listPSCommands = oldFile, oldList })
	psCacheFile = func() string { return cache }
	calls := make(chan bool, 2)
	listPSCommands = func() ([]string, error) { calls <- true; return []string{"Get-Item", "ls"}, nil }

	psCommands.Lock()
	oldNames, oldLoaded := psCommands.names, psCommands.loaded
	psCommands.names, psCommands.loaded = nil, false
	psCommands.Unlock()
	t.Cleanup(func() {
		psCommands.Lock()
		psCommands.names, psCommands.loaded = oldNames, oldLoaded
		psCommands.Unlock()
	})

	// Cache récent : lu tout de suite, PowerShell n'est pas lancé.
	os.WriteFile(cache, []byte("Get-Process\ndir\n"), 0o644)
	loadPSCommands()
	if knows, known := psKnows("get-process"); !known || !knows {
		t.Fatal("la liste en cache doit être lue")
	}
	select {
	case <-calls:
		t.Fatal("cache récent : pas besoin de PowerShell")
	case <-time.After(50 * time.Millisecond):
	}

	// Cache ancien : relu, puis rafraîchi en arrière-plan.
	psCommands.Lock()
	psCommands.loaded = false
	psCommands.Unlock()
	old := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(cache, old, old)
	loadPSCommands()
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("cache ancien : la liste doit être rafraîchie")
	}
	// Attendre la fin du rafraîchissement (liste en mémoire et fichier réécrit).
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, _ := os.ReadFile(cache)
		knows, _ := psKnows("Get-Item")
		if knows && strings.Contains(string(data), "Get-Item") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("la nouvelle liste doit être prise en compte et enregistrée")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestInstalledButNotInPathIsFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script sh")
	}
	s, out, errOut, _ := testShell(t)
	setPSCommands(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hurl"), []byte("#!/bin/sh\necho hurl $1\n"), 0o755)
	oldGoos, oldRefresh, oldFind, oldPath := goos, refreshPath, findInstalledExe, os.Getenv("PATH")
	t.Cleanup(func() { goos, refreshPath, findInstalledExe = oldGoos, oldRefresh, oldFind; os.Setenv("PATH", oldPath) })
	goos = "windows"
	refreshPath = func() bool { return false }
	findInstalledExe = func(name string) string {
		if name == "hurl" {
			return dir
		}
		return ""
	}
	if code := s.RunLine("hurl --version"); code != 0 || out.String() != "hurl --version\n" {
		t.Fatalf("code %d, sortie %q, erreurs %q", code, out, errOut)
	}
	if !strings.Contains(errOut.String(), "ajouté au PATH") {
		t.Fatalf("le terminal doit dire où il l'a trouvé : %q", errOut)
	}
}
