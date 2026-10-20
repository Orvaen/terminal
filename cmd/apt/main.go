// Command apt apporte les commandes apt de Linux à Windows, au-dessus de winget.
package main

import (
	"os"

	"github.com/Orvaen/terminal/internal/apt"
)

func main() {
	os.Exit(apt.Run(os.Args[1:]))
}
