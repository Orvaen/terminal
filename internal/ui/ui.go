// Package ui affiche les messages d'apt, en couleur quand le terminal le permet.
package ui

import (
	"fmt"
	"os"

	"github.com/Orvaen/terminal/internal/platform"
)

var color = os.Getenv("NO_COLOR") == "" && platform.EnableVirtualTerminal()

// ColorEnabled indique si les couleurs ANSI sont actives.
func ColorEnabled() bool { return color }

func paint(code, s string) string {
	if !color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Step annonce une étape, comme les en-têtes « ==> » des gestionnaires de paquets.
func Step(msg string) {
	fmt.Fprintln(os.Stdout, paint("1;34", "==>")+" "+paint("1", msg))
}

// Info affiche une information.
func Info(msg string) {
	fmt.Fprintln(os.Stdout, msg)
}

// Warn affiche un avertissement au format apt (W:).
func Warn(msg string) {
	fmt.Fprintln(os.Stderr, paint("1;33", "W:")+" "+msg)
}

// Errorf affiche une erreur au format apt (E:).
func Errorf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, paint("1;31", "E:")+" "+fmt.Sprintf(format, a...))
}
