package shell

// Scripts sous Windows.
//
// Windows ne sait lancer que des exécutables natifs : passer ./build.sh à CreateProcess donne
// « %1 n'est pas une application Win32 valide ». Comme Git Bash, le terminal lit donc la ligne
// shebang (#!/usr/bin/env bash, #!/bin/sh…) et lance l'interprète avec le script en argument.
// Un .sh sans shebang part sur bash, un .ps1 sur PowerShell. Sous Linux et macOS, le noyau gère
// lui-même le shebang : rien ne change.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// scriptEnv regroupe ce dont scriptCommand a besoin, pour pouvoir le tester sans Windows.
type scriptEnv struct {
	goos      string
	firstLine func(path string) string          // première ligne du fichier ("" si illisible)
	look      func(name string) (string, error) // exec.LookPath
	gitUnix   string                            // Git\usr\bin, ou ""
	exists    func(path string) bool
}

func defaultScriptEnv() scriptEnv {
	return scriptEnv{
		goos:      runtime.GOOS,
		firstLine: readFirstLine,
		look:      exec.LookPath,
		gitUnix:   gitUnixDir(func() (string, error) { return exec.LookPath("git") }, os.Getenv("ProgramFiles")),
		exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
	}
}

// commandFor prépare l'exécution de path avec args, en passant par un interprète si c'est un script.
func commandFor(path string, args []string) (*exec.Cmd, error) {
	prog, full, err := scriptCommand(path, args, defaultScriptEnv())
	if err != nil {
		return nil, err
	}
	return exec.Command(prog, full...), nil
}

// scriptCommand renvoie le programme réel à lancer et ses arguments.
func scriptCommand(path string, args []string, env scriptEnv) (string, []string, error) {
	if env.goos != "windows" {
		return path, args, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".com", ".bat", ".cmd":
		return path, args, nil
	case ".ps1":
		ps, err := firstFound(env, "pwsh", "powershell")
		if err != nil {
			return "", nil, fmt.Errorf("%s : PowerShell est introuvable pour lancer ce script", path)
		}
		return ps, append([]string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}, args...), nil
	}

	interp, interpArgs := parseShebang(env.firstLine(path))
	if interp == "" {
		if !strings.EqualFold(filepath.Ext(path), ".sh") {
			return path, args, nil // pas un script reconnu : on laisse Windows répondre
		}
		interp = "bash"
	}

	prog, err := findInterpreter(interp, env)
	if err != nil {
		return "", nil, fmt.Errorf("%s : ce script demande « %s », introuvable%s", path, interp, interpreterHint(interp))
	}
	full := append(append(interpArgs, scriptArg(interp, path)), args...)
	return prog, full, nil
}

// parseShebang lit « #!/usr/bin/env bash -e » → ("bash", ["-e"]) ; « #!/bin/sh » → ("sh", nil).
func parseShebang(line string) (string, []string) {
	line = strings.TrimPrefix(line, "\uFEFF")
	if !strings.HasPrefix(line, "#!") {
		return "", nil
	}
	fields := strings.Fields(strings.TrimSpace(line[2:]))
	if len(fields) == 0 {
		return "", nil
	}
	prog, rest := fields[0], fields[1:]
	if base(prog) == "env" {
		// env -S « bash -e » : on ignore les options d'env et on prend le premier vrai mot.
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return "", nil
		}
		prog, rest = rest[0], rest[1:]
	}
	if len(rest) == 0 {
		rest = nil
	}
	return base(prog), rest
}

// base : nom de l'interprète sans dossier ni « .exe » (/usr/bin/bash → bash).
func base(p string) string {
	p = p[strings.LastIndexAny(p, `/\`)+1:]
	return strings.TrimSuffix(strings.ToLower(p), ".exe")
}

// findInterpreter cherche l'interprète. Pour bash et sh, celui de Git pour Windows passe avant
// C:\Windows\System32\bash.exe, qui est le lanceur de WSL : il exécuterait le script dans Linux,
// sans les programmes Windows (java, go…) que le script appelle.
func findInterpreter(name string, env scriptEnv) (string, error) {
	if name == "bash" || name == "sh" {
		if env.gitUnix != "" {
			p := filepath.Join(env.gitUnix, name+".exe")
			if env.exists(p) {
				return p, nil
			}
		}
		if p, err := env.look(name); err == nil && !isWSLLauncher(p) {
			return p, nil
		}
		if name == "sh" {
			return findInterpreter("bash", env)
		}
		return env.look(name) // dernier recours : WSL
	}
	switch name {
	case "python3", "python":
		return firstFound(env, name, "python", "py")
	case "node", "nodejs":
		return firstFound(env, "node")
	}
	return env.look(name)
}

func isWSLLauncher(p string) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(p, `\`, "/")), "/windows/system32/")
}

func firstFound(env scriptEnv, names ...string) (string, error) {
	var last error
	for _, n := range names {
		p, err := env.look(n)
		if err == nil {
			return p, nil
		}
		last = err
	}
	return "", last
}

// scriptArg : bash et sh (MSYS) comprennent C:/… mais pas toujours C:\… dans les chemins.
func scriptArg(interp, path string) string {
	if interp == "bash" || interp == "sh" {
		return strings.ReplaceAll(path, `\`, "/")
	}
	return path
}

func interpreterHint(interp string) string {
	switch interp {
	case "bash", "sh":
		return " (installe Git pour Windows : apt install git, ou lance la version .bat du script s'il y en a une)"
	case "python3", "python":
		return " (apt install python)"
	case "node":
		return " (apt install nodejs)"
	}
	return ""
}

// readFirstLine lit au plus 512 octets du fichier et renvoie sa première ligne.
func readFirstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := bufio.NewReader(f).Read(buf)
	line := buf[:n]
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimRight(string(line), "\r")
}
