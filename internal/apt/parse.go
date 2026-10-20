package apt

import "strings"

// Invocation est une ligne de commande apt analysée.
type Invocation struct {
	Command    string
	Packages   []string
	Yes        bool // -y : accepté pour la compatibilité, les accords sont toujours acceptés
	Upgradable bool // apt list --upgradable
	Pause      bool // usage interne : attendre Entrée avant de fermer la fenêtre admin
}

// Parse analyse les arguments comme apt : les options peuvent être placées n'importe où.
func Parse(args []string) Invocation {
	var inv Invocation
	for _, a := range args {
		switch a {
		case "-y", "--yes", "--assume-yes":
			inv.Yes = true
			continue
		case "--upgradable":
			inv.Upgradable = true
			continue
		case "--pause":
			inv.Pause = true
			continue
		}
		if inv.Command == "" {
			inv.Command = strings.ToLower(a)
			continue
		}
		inv.Packages = append(inv.Packages, a)
	}
	return inv
}
