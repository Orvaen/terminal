// Command apt apporte les commandes apt de Linux à Windows, au-dessus de winget.
package main

import (
	"os"

	"github.com/Orvaen/terminal/internal/apt"
	"github.com/Orvaen/terminal/internal/platform"
)

func main() {
	// Console en UTF-8 le temps de la commande, pour les accents de winget et des autres programmes.
	restore := platform.EnableUTF8Console()
	code := apt.Run(os.Args[1:])
	restore()
	os.Exit(code)
}
