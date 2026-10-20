// Command docker relaie les commandes docker vers le Docker léger d'Orvaen Terminal
// (Docker Engine dans une mini machine Alpine sous WSL2), pour l'utiliser aussi depuis
// PowerShell, cmd ou VS Code.
package main

import (
	"os"

	"github.com/Orvaen/terminal/internal/docker"
	"github.com/Orvaen/terminal/internal/platform"
)

func main() {
	// Console en UTF-8 le temps de la commande, pour les accents de winget et des autres programmes.
	restore := platform.EnableUTF8Console()
	code := docker.Run(os.Args[1:])
	restore()
	os.Exit(code)
}
