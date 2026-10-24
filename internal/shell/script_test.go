package shell

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseShebang(t *testing.T) {
	cases := []struct {
		line string
		prog string
		args []string
	}{
		{"#!/usr/bin/env sh", "sh", nil},
		{"#!/usr/bin/env bash", "bash", nil},
		{"#!/bin/bash -e", "bash", []string{"-e"}},
		{"#! /bin/sh", "sh", nil},
		{"#!/usr/bin/env -S bash -eu", "bash", []string{"-eu"}},
		{"#!/usr/bin/python3", "python3", nil},
		{"\uFEFF#!/usr/bin/env node", "node", nil},
		{`#!C:\Git\usr\bin\bash.exe`, "bash", nil},
		{"echo pas de shebang", "", nil},
		{"#!", "", nil},
		{"", "", nil},
	}
	for _, c := range cases {
		prog, args := parseShebang(c.line)
		if prog != c.prog || !reflect.DeepEqual(args, c.args) {
			t.Errorf("parseShebang(%q) = %q %v, attendu %q %v", c.line, prog, args, c.prog, c.args)
		}
	}
}

// fakeWindows simule un Windows avec Git pour Windows et WSL installés.
func fakeWindows(lines map[string]string, onPath map[string]string, git bool) scriptEnv {
	gitDir := `C:\Program Files\Git\usr\bin`
	return scriptEnv{
		goos:      "windows",
		firstLine: func(p string) string { return lines[p] },
		look: func(name string) (string, error) {
			if p, ok := onPath[name]; ok {
				return p, nil
			}
			return "", errors.New("introuvable")
		},
		gitUnix: map[bool]string{true: gitDir, false: ""}[git],
		exists: func(p string) bool {
			return git && (p == filepath.Join(gitDir, "bash.exe") || p == filepath.Join(gitDir, "sh.exe"))
		},
	}
}

func TestScriptCommandWindows(t *testing.T) {
	gitBash := filepath.Join(`C:\Program Files\Git\usr\bin`, "bash.exe")
	gitSh := filepath.Join(`C:\Program Files\Git\usr\bin`, "sh.exe")
	wslBash := `C:\Windows\System32\bash.exe`
	path := map[string]string{"bash": wslBash, "powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		"python": `C:\Python312\python.exe`}
	lines := map[string]string{
		"./build.sh":  "#!/usr/bin/env sh",
		"./deploy.sh": "echo sans shebang",
		"./outil":     "#!/bin/bash -e",
		"./gen.py":    "#!/usr/bin/env python3",
		"./notes.txt": "bonjour",
	}
	env := fakeWindows(lines, path, true)

	cases := []struct {
		path string
		prog string
		args []string
	}{
		{"./build.sh", gitSh, []string{"./build.sh", "--vite"}},
		{"./deploy.sh", gitBash, []string{"./deploy.sh", "--vite"}},
		{"./outil", gitBash, []string{"-e", "./outil", "--vite"}},
		{"./gen.py", `C:\Python312\python.exe`, []string{"./gen.py", "--vite"}},
		{`C:\Jeux\build.sh`, gitBash, []string{"C:/Jeux/build.sh", "--vite"}},
		{"./build.bat", "./build.bat", []string{"--vite"}},
		{`C:\Go\bin\go.exe`, `C:\Go\bin\go.exe`, []string{"--vite"}},
		{"./notes.txt", "./notes.txt", []string{"--vite"}},
		{"./tache.ps1", path["powershell"], []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "./tache.ps1", "--vite"}},
	}
	for _, c := range cases {
		prog, args, err := scriptCommand(c.path, []string{"--vite"}, env)
		if err != nil {
			t.Errorf("%s : erreur inattendue %v", c.path, err)
			continue
		}
		if prog != c.prog || !reflect.DeepEqual(args, c.args) {
			t.Errorf("%s → %q %v, attendu %q %v", c.path, prog, args, c.prog, c.args)
		}
	}
}

func TestScriptCommandPrefersGitBashOverWSL(t *testing.T) {
	// Sans Git : le bash du PATH qui n'est pas WSL passe avant WSL…
	env := fakeWindows(map[string]string{"./a.sh": "#!/bin/bash"},
		map[string]string{"bash": `C:\msys64\usr\bin\bash.exe`}, false)
	if prog, _, _ := scriptCommand("./a.sh", nil, env); prog != `C:\msys64\usr\bin\bash.exe` {
		t.Errorf("bash attendu depuis msys64, obtenu %q", prog)
	}
	// …et WSL n'est qu'un dernier recours.
	env = fakeWindows(map[string]string{"./a.sh": "#!/bin/bash"},
		map[string]string{"bash": `C:\Windows\System32\bash.exe`}, false)
	if prog, _, _ := scriptCommand("./a.sh", nil, env); prog != `C:\Windows\System32\bash.exe` {
		t.Errorf("WSL attendu en dernier recours, obtenu %q", prog)
	}
}

func TestScriptCommandMissingInterpreter(t *testing.T) {
	env := fakeWindows(map[string]string{"./build.sh": "#!/usr/bin/env sh"}, map[string]string{}, false)
	_, _, err := scriptCommand("./build.sh", nil, env)
	if err == nil || !strings.Contains(err.Error(), "Git pour Windows") {
		t.Fatalf("message d'aide attendu, obtenu %v", err)
	}
}

func TestScriptCommandOtherSystemsUnchanged(t *testing.T) {
	env := fakeWindows(map[string]string{"./build.sh": "#!/bin/sh"}, nil, true)
	env.goos = "linux"
	prog, args, err := scriptCommand("./build.sh", []string{"x"}, env)
	if err != nil || prog != "./build.sh" || !reflect.DeepEqual(args, []string{"x"}) {
		t.Fatalf("Linux : le noyau gère le shebang, rien ne doit changer (%q %v %v)", prog, args, err)
	}
}

func TestReadFirstLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(p, []byte("#!/usr/bin/env sh\r\necho ok\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFirstLine(p); got != "#!/usr/bin/env sh" {
		t.Errorf("première ligne = %q (fin de ligne Windows mal retirée ?)", got)
	}
	if got := readFirstLine(filepath.Join(dir, "absent")); got != "" {
		t.Errorf("fichier absent → %q", got)
	}
}
