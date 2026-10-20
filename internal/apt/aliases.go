package apt

import (
	"sort"
	"strings"
)

// aliases associe des noms courts façon Linux aux identifiants winget exacts.
// Tout nom absent de la liste est transmis tel quel à winget, qui fait sa propre recherche.
var aliases = map[string]string{
	"7zip":    "7zip.7zip",
	"chrome":  "Google.Chrome",
	"code":    "Microsoft.VisualStudioCode",
	"curl":    "cURL.cURL",
	"discord": "Discord.Discord",
	// « docker » installe le Docker léger (voir isDocker) ; Docker Desktop reste disponible sous ce nom.
	"docker-desktop": "Docker.DockerDesktop",
	"firefox":        "Mozilla.Firefox",
	"gh":             "GitHub.cli",
	"git":            "Git.Git",
	"go":             "GoLang.Go",
	"golang":         "GoLang.Go",
	"neovim":         "Neovim.Neovim",
	"nvim":           "Neovim.Neovim",
	"node":           "OpenJS.NodeJS.LTS",
	"node-current":   "OpenJS.NodeJS",
	"nodejs":         "OpenJS.NodeJS.LTS",
	"pwsh":           "Microsoft.PowerShell",
	"python":         "Python.Python.3.12",
	"python3":        "Python.Python.3.12",
	"rust":           "Rustlang.Rustup",
	"rustup":         "Rustlang.Rustup",
	"terminal":       "Microsoft.WindowsTerminal",
	"vlc":            "VideoLAN.VLC",
	"vscode":         "Microsoft.VisualStudioCode",
}

// Resolve renvoie l'identifiant winget d'un alias connu, et true s'il s'agit d'un identifiant exact.
func Resolve(name string) (string, bool) {
	if id, ok := aliases[strings.ToLower(name)]; ok {
		return id, true
	}
	return name, false
}

// AliasNames renvoie la liste triée des alias connus.
func AliasNames() []string {
	names := make([]string, 0, len(aliases))
	for n := range aliases {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
