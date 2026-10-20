package apt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// docs/paquets.md est généré depuis la table des noms (aliases.go), les outils installés par apt
// (tools.go) et les notes Windows (resolve.go). Pour le régénérer :
//
//	UPDATE_DOCS=1 go test ./internal/apt -run TestPackagesDoc
func TestPackagesDocUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "paquets.md")
	want := renderPackagesDoc()
	if os.Getenv("UPDATE_DOCS") != "" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != want {
		t.Fatal("docs/paquets.md n'est plus à jour : UPDATE_DOCS=1 go test ./internal/apt -run TestPackagesDoc")
	}
}

func code(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "`" + n + "`"
	}
	return strings.Join(out, ", ")
}

func renderPackagesDoc() string {
	var b strings.Builder
	total := 0
	for _, g := range aliasGroups {
		total += len(g.Entries)
	}
	fmt.Fprintf(&b, `# Paquets connus

[← apt](apt.md)

<!-- Fichier généré depuis internal/apt (aliases.go, tools.go, resolve.go) : ne pas modifier à la main.
     Pour le régénérer : UPDATE_DOCS=1 go test ./internal/apt -run TestPackagesDoc -->

`+"`apt install`"+` accepte les noms Linux (Debian, Ubuntu, Arch, Homebrew) de cette page. Les identifiants winget
ont été vérifiés dans le dépôt des manifestes winget. Pour un nom absent, `+"`apt`"+` cherche dans winget le logiciel
qui porte ce nom court ou fournit cette commande, et te fait choisir si plusieurs correspondent : voir
[apt › Noms de paquets](apt.md#noms-de-paquets).

%d noms pour %d logiciels, plus les outils ci-dessous.

## Installés par apt lui-même

Ces outils ne sont pas dans winget (ou demandent un réglage que winget ne fait pas seul). `+"`apt`"+` les installe
depuis leur source officielle. Les outils de développement sont rangés dans `+"`%%LOCALAPPDATA%%\\OrvaenTerminal\\outils`"+`,
et leur dossier est ajouté à ton PATH. `+"`apt upgrade`"+` les met à jour, `+"`apt remove`"+` les désinstalle.

| Noms | Ce qui est installé | Source | A besoin de |
|---|---|---|---|
| `+"`docker`, `docker.io`, `docker-ce`, `docker-compose`"+` | Docker Engine et Compose, sans Docker Desktop | [Docker léger](docker.md) | WSL2 |
| `+"`terminal`"+` (avec `+"`apt upgrade`"+`) | Orvaen Terminal | releases GitHub, somme SHA-256 vérifiée | |
`, len(aliases), total)
	for _, tool := range extTools {
		needs := ""
		if tool.NeedsPkg != "" {
			needs = "`" + tool.NeedsPkg + "`"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", code(append([]string{tool.Name}, tool.Aliases...)), tool.Label, tool.Source, needs)
	}
	var be []string
	for n := range buildEssentialNames {
		be = append(be, n)
	}
	sort.Slice(be, func(i, j int) bool {
		return be[i] == "build-essential" || (be[j] != "build-essential" && be[i] < be[j])
	})
	fmt.Fprintf(&b, "| %s | Compilateur C/C++ MSVC : `%s` avec la charge de travail « Développement Desktop en C++ » | winget | |\n", code(be), msvcID)

	b.WriteString("\n## Déjà dans Windows\n\nPour ces paquets, `apt` explique l'équivalent au lieu d'installer autre chose.\n\n| Nom | Ce qu'apt répond |\n|---|---|\n")
	var notes []string
	for n := range windowsNotes {
		notes = append(notes, n)
	}
	sort.Strings(notes)
	for _, n := range notes {
		fmt.Fprintf(&b, "| `%s` | %s |\n", n, strings.ReplaceAll(windowsNotes[n], "|", "\\|"))
	}

	for _, g := range aliasGroups {
		fmt.Fprintf(&b, "\n## %s\n\n| Noms | Identifiant winget |\n|---|---|\n", g.Title)
		for _, e := range g.Entries {
			fmt.Fprintf(&b, "| %s | `%s` |\n", code(e.Names), e.ID)
		}
	}
	return b.String()
}
