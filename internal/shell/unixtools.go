package shell

// Outils Unix de Git pour Windows.
//
// Git pour Windows embarque sh, bash, grep, sed, awk, id… dans Git\usr\bin, mais ne met dans le
// PATH Windows que Git\cmd. Git Bash, lui, les a dans son PATH : c'est ce qui permet à make de
// trouver un sh.exe et d'exécuter les recettes des Makefile (mkdir -p, [ ], $(shell id -u)…).
// Sans eux, make se rabat sur cmd.exe et les recettes échouent.
//
// Le terminal ajoute donc Git\usr\bin à la fin de son PATH : ces outils comblent ce qui manque
// sans masquer les commandes Windows du même nom (find, sort…), qui restent prioritaires.
// Il règle aussi SHELL sur ce sh (voir setMakeShell).

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// gitUnixDir trouve le dossier usr\bin de Git pour Windows, ou "" s'il est absent.
func gitUnixDir(lookGit func() (string, error), programFiles string) string {
	var candidates []string
	if git, err := lookGit(); err == nil {
		// …\Git\cmd\git.exe, …\Git\bin\git.exe ou …\Git\mingw64\bin\git.exe
		dir := filepath.Dir(git)
		for i := 0; i < 3; i++ {
			dir = filepath.Dir(dir)
			candidates = append(candidates, dir)
		}
	}
	if programFiles != "" {
		candidates = append(candidates, filepath.Join(programFiles, "Git"))
	}
	for _, root := range candidates {
		usr := filepath.Join(root, "usr", "bin")
		if _, err := os.Stat(filepath.Join(usr, "sh.exe")); err == nil {
			return usr
		}
	}
	return ""
}

// withUnixTools renvoie le PATH complété par dir (en dernier), sans doublon.
func withUnixTools(path, dir string) string {
	if dir == "" {
		return path
	}
	for _, p := range filepath.SplitList(path) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			return path
		}
	}
	if path == "" {
		return dir
	}
	return path + string(os.PathListSeparator) + dir
}

// addUnixTools complète le PATH du terminal (et donc des programmes qu'il lance).
func addUnixTools() {
	if runtime.GOOS != "windows" || os.Getenv("ORVAEN_NO_UNIX_TOOLS") != "" {
		return
	}
	dir := gitUnixDir(func() (string, error) { return exec.LookPath("git") }, os.Getenv("ProgramFiles"))
	if dir != "" {
		os.Setenv("PATH", withUnixTools(os.Getenv("PATH"), dir))
		setMakeShell(dir)
	}
}

// setMakeShell règle SHELL sur le sh de Git, comme Git Bash. Sous Windows, GNU make lit cette variable
// d'environnement : il exécute alors les recettes avec sh quel que soit le make trouvé en premier
// dans le PATH (un ancien make, celui de Free Pascal…), au lieu de se rabattre sur cmd.exe, qui ne
// comprend ni { … }, ni [ … ], ni $(shell …).
func setMakeShell(dir string) {
	if os.Getenv("SHELL") != "" {
		return // déjà choisi par l'utilisateur
	}
	sh := filepath.Join(dir, "sh.exe")
	if _, err := os.Stat(sh); err == nil {
		// Les barres obliques évitent qu'un make ancien interprète les \ du chemin.
		os.Setenv("SHELL", filepath.ToSlash(sh))
	}
}
