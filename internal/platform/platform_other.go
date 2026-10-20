//go:build !windows

// Package platform regroupe ce qui dépend du système : droits admin, élévation et console.
// Hors Windows, ces fonctions servent seulement à compiler et tester le projet.
package platform

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// IsElevated indique si le processus tourne en root.
func IsElevated() bool { return os.Geteuid() == 0 }

// EnableVirtualTerminal : les terminaux Unix gèrent déjà les couleurs ANSI.
func EnableVirtualTerminal() bool { return true }

// TerminalWidth lit $COLUMNS, 80 par défaut.
func TerminalWidth() int {
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w >= 20 {
		return w
	}
	return 80
}

// IsHidden : hors Windows, seuls les noms commençant par un point sont cachés (géré par l'appelant).
func IsHidden(fs.FileInfo) bool { return false }

// EnableRawInput : la saisie touche par touche n'est gérée que sous Windows (repli sur la saisie par ligne).
func EnableRawInput() (restore func(), ok bool) { return nil, false }

// NativeSudoInline : le sudo de Windows n'existe pas ici.
func NativeSudoInline() bool { return false }

// RunSelfElevated n'est pas pris en charge hors Windows.
func RunSelfElevated(args ...string) int { return 1 }

// KillTree arrête un processus (hors Windows, sans ses enfants : utilisé seulement pour les tests).
func KillTree(p *os.Process) error { return p.Kill() }

// RegistryPathDirs : hors Windows, pas de registre.
func RegistryPathDirs() []string { return nil }

// EnableUTF8Console : les terminaux Unix sont déjà en UTF-8.
func EnableUTF8Console() (restore func()) { return func() {} }

// AddUserPath : hors Windows, rien à faire (utilisé seulement pour les tests).
func AddUserPath(dir string) error { return nil }

// RemoveUserPath : hors Windows, rien à faire.
func RemoveUserPath(dir string) error { return nil }

// FindInstalledExe : hors Windows, pas de registre des programmes installés.
func FindInstalledExe(name string) string { return "" }

// PathEntry : un dossier du PATH enregistré dans Windows, tel qu'écrit et développé.
type PathEntry struct {
	Raw string
	Dir string
}

// RegistryPath : hors Windows, pas de registre.
func RegistryPath(scope string) []PathEntry { return nil }

// WritePath : hors Windows, rien à écrire.
func WritePath(scope string, raw []string) error { return errNotWindows }

var errNotWindows = errors.New("disponible seulement sous Windows")

// TerminalHeight lit $LINES, 24 par défaut.
func TerminalHeight() int {
	if h, err := strconv.Atoi(os.Getenv("LINES")); err == nil && h >= 5 {
		return h
	}
	return 24
}

// Locale : la langue de l'utilisateur au format de LANG (fr_BE), « C » si inconnue.
func Locale() string {
	for _, k := range []string{"LC_ALL", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if l, _, _ := strings.Cut(v, "."); l != "" {
				return l
			}
		}
	}
	return "C"
}
