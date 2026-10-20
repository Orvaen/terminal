//go:build !windows

// Package platform regroupe ce qui dépend du système : droits admin, élévation et console.
// Hors Windows, ces fonctions servent seulement à compiler et tester le projet.
package platform

import "os"

// IsElevated indique si le processus tourne en root.
func IsElevated() bool { return os.Geteuid() == 0 }

// EnableVirtualTerminal : les terminaux Unix gèrent déjà les couleurs ANSI.
func EnableVirtualTerminal() bool { return true }

// RunSelfElevated n'est pas pris en charge hors Windows.
func RunSelfElevated(args ...string) int { return 1 }
