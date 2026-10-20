package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// RefreshPath ajoute au PATH du processus les dossiers que les installeurs ont enregistrés depuis
// son démarrage (Node, Go, Hurl…), pour s'en servir sans rouvrir le terminal. Renvoie true si le
// PATH a changé.
func RefreshPath() bool {
	cur := os.Getenv("PATH")
	seen := map[string]bool{}
	for _, p := range filepath.SplitList(cur) {
		seen[strings.ToLower(filepath.Clean(p))] = true
	}
	parts := []string{cur}
	for _, d := range RegistryPathDirs() {
		if k := strings.ToLower(filepath.Clean(d)); d != "" && !seen[k] {
			seen[k] = true
			parts = append(parts, d)
		}
	}
	if len(parts) == 1 {
		return false
	}
	os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
	return true
}

// SamePath compare deux dossiers comme Windows : sans tenir compte de la casse ni du \ final.
func SamePath(a, b string) bool {
	norm := func(p string) string {
		p = strings.TrimRight(filepath.Clean(strings.TrimSpace(p)), `\/`)
		return strings.ToLower(p)
	}
	return a != "" && b != "" && norm(a) == norm(b)
}
