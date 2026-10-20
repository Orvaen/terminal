// Command docker relaie les commandes docker vers le Docker léger d'Orvaen Terminal
// (Docker Engine dans une mini machine Alpine sous WSL2), pour l'utiliser aussi depuis
// PowerShell, cmd ou VS Code.
package main

import (
	"os"

	"github.com/Orvaen/terminal/internal/docker"
)

func main() {
	os.Exit(docker.Run(os.Args[1:]))
}
