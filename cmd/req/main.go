// Command req envoie les requêtes HTTP décrites dans un fichier .http et vérifie les réponses.
package main

import (
	"os"

	"github.com/Orvaen/terminal/internal/req"
)

func main() {
	os.Exit(req.Run(os.Args[1:], os.Stdout))
}
