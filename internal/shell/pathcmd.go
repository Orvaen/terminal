package shell

// path : voir et ranger le PATH.
//
// Quand deux programmes portent le même nom (deux make.exe, deux python.exe), c'est le premier
// dossier du PATH qui gagne. « path qui make » montre lequel ; « path monter » et « path masquer »
// changent l'ordre pour le terminal et les programmes qu'il lance, sans toucher à Windows ni
// demander de droits ; « path nettoyer » retire de Windows les doublons et les dossiers disparus.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Orvaen/terminal/internal/platform"
)

// Points d'injection, remplacés dans les tests.
var (
	registryPath = platform.RegistryPath
	writePath    = platform.WritePath
	versionLine  = func(path string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, path, "--version").CombinedOutput()
		for _, l := range strings.Split(string(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				if len(l) > 60 {
					l = l[:60] + "…"
				}
				return l
			}
		}
		return ""
	}
)

// pathPrefs : l'ordre choisi pour le terminal, gardé dans terminal-config.json.
type pathPrefs struct {
	First  []string `json:"path_avant,omitempty"`   // dossiers mis en tête, dans cet ordre
	Hidden []string `json:"path_masques,omitempty"` // dossiers retirés du PATH du terminal
}

func loadPathPrefs() pathPrefs {
	var p pathPrefs
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}

func savePathPrefs(p pathPrefs) error {
	return saveConfigKeys(map[string]any{"path_avant": p.First, "path_masques": p.Hidden})
}

// saveConfigKeys met à jour des clés de terminal-config.json en gardant les autres (une liste vide retire la clé).
func saveConfigKeys(kv map[string]any) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	all := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	for k, v := range kv {
		if l, ok := v.([]string); ok && len(l) == 0 {
			delete(all, k)
			continue
		}
		all[k] = v
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// applyPathPrefs applique l'ordre choisi au PATH du terminal (au démarrage, et après chaque changement).
func applyPathPrefs() {
	p := loadPathPrefs()
	if len(p.First) == 0 && len(p.Hidden) == 0 {
		return
	}
	var kept []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d == "" || containsPath(p.Hidden, d) || containsPath(p.First, d) {
			continue
		}
		kept = append(kept, d)
	}
	var first []string
	for _, d := range p.First {
		if !containsPath(p.Hidden, d) {
			first = append(first, d)
		}
	}
	os.Setenv("PATH", strings.Join(append(first, kept...), string(os.PathListSeparator)))
	resetCommandNames()
}

func containsPath(list []string, dir string) bool {
	for _, d := range list {
		if platform.SamePath(d, dir) {
			return true
		}
	}
	return false
}

func removePath(list []string, dir string) []string {
	var out []string
	for _, d := range list {
		if !platform.SamePath(d, dir) {
			out = append(out, d)
		}
	}
	return out
}

func pathDirs() []string {
	var dirs []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func dirExists(d string) bool {
	info, err := os.Stat(d)
	return err == nil && info.IsDir()
}

// origin : d'où vient un dossier du PATH du terminal.
func origin(dir string, system, user []platform.PathEntry, prefs pathPrefs) string {
	if containsPath(prefs.First, dir) {
		return "mis en tête (path monter)"
	}
	for _, e := range system {
		if platform.SamePath(e.Dir, dir) {
			return "Windows, système"
		}
	}
	for _, e := range user {
		if platform.SamePath(e.Dir, dir) {
			return "Windows, utilisateur"
		}
	}
	return "ajouté par le terminal ou la session"
}

// executableIn : le programme name dans dir, en tenant compte de PATHEXT sous Windows.
func executableIn(dir, name string) string {
	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = []string{".exe", ".cmd", ".bat", ".com", ""}
		if strings.Contains(name, ".") {
			exts = []string{""}
		}
	}
	for _, ext := range exts {
		p := filepath.Join(dir, name+ext)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			continue
		}
		return p
	}
	return ""
}

// matches : toutes les versions d'un programme dans le PATH, dans l'ordre (la première gagne).
func matches(name string) []string {
	var out []string
	var infos []os.FileInfo
next:
	for _, d := range pathDirs() {
		p := executableIn(d, name)
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		// Le même fichier vu par deux chemins (lien, /bin → /usr/bin) ne compte qu'une fois.
		for _, o := range infos {
			if os.SameFile(o, info) {
				continue next
			}
		}
		infos = append(infos, info)
		out = append(out, p)
	}
	return out
}

// conflictCandidates : les commandes de développement où deux versions se gênent souvent.
var conflictCandidates = []string{
	"make", "sh", "bash", "python", "python3", "pip", "node", "npm", "git", "gcc", "g++", "clang", "cmake",
	"java", "javac", "go", "cargo", "php", "ruby", "perl", "curl", "tar", "ssh", "find", "sort", "docker",
	"code", "deno", "bun", "dotnet", "mvn", "gradle", "hurl", "jq", "wget",
}

func runPath(s *Shell, args []string) int {
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
		args = args[1:]
	}
	switch sub {
	case "", "liste", "list":
		return pathList(s)
	case "qui", "which":
		if len(args) != 1 {
			s.errorf("usage : path qui <commande>   (ex. path qui make)")
			return 1
		}
		return pathWho(s, args[0])
	case "conflits", "conflicts":
		return pathConflicts(s)
	case "monter", "avant", "up":
		return pathMove(s, args, true)
	case "masquer", "cacher", "hide":
		return pathMove(s, args, false)
	case "reset":
		if err := savePathPrefs(pathPrefs{}); err != nil {
			s.errorf("path: %v", err)
			return 1
		}
		fmt.Fprintln(s.out, "Ordre du terminal oublié. Il s'appliquera aux prochaines fenêtres (celle-ci garde son PATH actuel).")
		return 0
	case "nettoyer", "clean":
		yes := len(args) > 0 && (args[0] == "-y" || args[0] == "--yes")
		return pathClean(s, yes)
	case "aide", "help", "-h", "--help":
		fmt.Fprint(s.out, pathHelp)
		return 0
	}
	s.errorf("path: sous-commande inconnue : %s (voir path aide)", sub)
	return 1
}

const pathHelp = `path — voir et ranger le PATH

  path                     les dossiers du PATH, dans l'ordre, avec leur origine
  path qui <commande>      toutes les versions d'un programme ; la première gagne
  path conflits            les outils de développement présents en plusieurs versions
  path monter <n°|dossier> met ce dossier en tête, pour le terminal
  path monter <cmd> <n°>   met en tête le dossier de la n-ième version (ex. path monter make 2)
  path masquer <n°|dossier> retire ce dossier du PATH du terminal
  path reset               oublie l'ordre choisi pour le terminal
  path nettoyer [-y]       retire de Windows les doublons et les dossiers disparus
                           (PATH utilisateur ; aussi le PATH système avec sudo)

monter et masquer valent pour le terminal et les programmes qu'il lance (make, npm…),
sans toucher à Windows ni demander de droits. Le choix est gardé pour les prochaines fenêtres.
`

func pathList(s *Shell) int {
	system, user := registryPath("system"), registryPath("user")
	prefs := loadPathPrefs()
	seen := map[string]int{}
	dead, dup := 0, 0
	fmt.Fprintln(s.out, "PATH du terminal (le premier dossier qui contient un programme gagne) :")
	for i, d := range pathDirs() {
		key := strings.ToLower(strings.TrimRight(filepath.Clean(d), `\/`))
		note := s.paint("2", origin(d, system, user, prefs))
		switch {
		case !dirExists(d):
			note = s.paint("31", "✗ n'existe pas") + "  " + note
			dead++
		case seen[key] > 0:
			note = s.paint("33", fmt.Sprintf("doublon du n°%d", seen[key])) + "  " + note
			dup++
		}
		if seen[key] == 0 {
			seen[key] = i + 1
		}
		fmt.Fprintf(s.out, "%3d  %s\n     %s\n", i+1, d, note)
	}
	if len(prefs.Hidden) > 0 {
		fmt.Fprintf(s.out, "\nMasqués pour le terminal : %s\n", strings.Join(prefs.Hidden, ", "))
	}
	fmt.Fprintln(s.out)
	if dead+dup > 0 {
		fmt.Fprintf(s.out, "%d dossier(s) disparu(s), %d doublon(s) : path nettoyer pour les retirer de Windows.\n", dead, dup)
	}
	fmt.Fprintln(s.out, "Quel programme gagne : path qui <commande>   Tout : path aide")
	return 0
}

func pathWho(s *Shell, name string) int {
	found := matches(name)
	if len(found) == 0 {
		s.errorf("%s : introuvable dans le PATH", name)
		return 1
	}
	for i, p := range found {
		mark := "   "
		if i == 0 {
			mark = s.paint("1;32", "→  ")
		}
		ver := versionLine(p)
		fmt.Fprintf(s.out, "%s%d. %s\n", mark, i+1, p)
		if ver != "" {
			fmt.Fprintf(s.out, "      %s\n", s.paint("2", ver))
		}
	}
	if len(found) > 1 {
		fmt.Fprintf(s.out, "\nC'est le n°1 qui est utilisé. Pour utiliser le n°2 : path monter %s 2\n", name)
	}
	return 0
}

func pathConflicts(s *Shell) int {
	n := 0
	for _, name := range conflictCandidates {
		found := matches(name)
		if len(found) < 2 {
			continue
		}
		n++
		fmt.Fprintf(s.out, "%s (%d versions) — utilisé : %s\n", s.paint("1", name), len(found), found[0])
		for _, p := range found[1:] {
			fmt.Fprintf(s.out, "      ignoré : %s\n", p)
		}
	}
	if n == 0 {
		fmt.Fprintln(s.out, "Aucun conflit parmi les outils de développement courants.")
		return 0
	}
	fmt.Fprintln(s.out, "\nPour voir les versions et choisir : path qui <commande>")
	return 0
}

// target : le dossier désigné par un n° de « path », un chemin, ou « <commande> <n°> » de « path qui ».
func target(args []string) (string, error) {
	dirs := pathDirs()
	switch len(args) {
	case 1:
		if n, err := strconv.Atoi(args[0]); err == nil {
			if n < 1 || n > len(dirs) {
				return "", fmt.Errorf("n° %d hors de la liste (1 à %d)", n, len(dirs))
			}
			return dirs[n-1], nil
		}
		d := convertPath(args[0], runtime.GOOS)
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		if !dirExists(d) {
			return "", fmt.Errorf("%s n'est pas un dossier", args[0])
		}
		return d, nil
	case 2:
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return "", fmt.Errorf("usage : path monter %s <n°>", args[0])
		}
		found := matches(args[0])
		if n < 1 || n > len(found) {
			return "", fmt.Errorf("%s : %d version(s) trouvée(s), pas de n°%d (voir path qui %s)", args[0], len(found), n, args[0])
		}
		return filepath.Dir(found[n-1]), nil
	}
	return "", fmt.Errorf("usage : path monter <n°|dossier> ou path monter <commande> <n°>")
}

func pathMove(s *Shell, args []string, up bool) int {
	dir, err := target(args)
	if err != nil {
		s.errorf("path: %v", err)
		return 1
	}
	prefs := loadPathPrefs()
	if up {
		prefs.First = append([]string{dir}, removePath(prefs.First, dir)...)
		prefs.Hidden = removePath(prefs.Hidden, dir)
	} else {
		prefs.Hidden = append(removePath(prefs.Hidden, dir), dir)
		prefs.First = removePath(prefs.First, dir)
	}
	if err := savePathPrefs(prefs); err != nil {
		s.errorf("path: %v", err)
		return 1
	}
	applyPathPrefs()
	if up {
		fmt.Fprintf(s.out, "%s passe en tête du PATH du terminal (gardé pour les prochaines fenêtres).\n", dir)
		if len(args) == 2 {
			fmt.Fprintf(s.out, "%s utilisé maintenant : %s\n", args[0], firstOr(matches(args[0]), "?"))
		}
	} else {
		fmt.Fprintf(s.out, "%s est masqué pour le terminal (path monter %s pour le remettre).\n", dir, quoteIfSpace(dir))
	}
	return 0
}

func firstOr(l []string, def string) string {
	if len(l) > 0 {
		return l[0]
	}
	return def
}

func quoteIfSpace(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

// cleanEntries : les entrées à garder (sans doublon ni dossier disparu) et celles retirées.
// Les doublons sont comparés après développement des variables (%SystemRoot% = C:\Windows).
func cleanEntries(entries []platform.PathEntry, already []platform.PathEntry, exists func(string) bool) (keep []string, removed []string) {
	seen := append([]platform.PathEntry{}, already...)
	for _, e := range entries {
		dup := false
		for _, o := range seen {
			if platform.SamePath(o.Dir, e.Dir) {
				dup = true
				break
			}
		}
		switch {
		case dup:
			removed = append(removed, e.Raw+"  (doublon)")
		case !exists(e.Dir):
			removed = append(removed, e.Raw+"  (n'existe plus)")
		default:
			keep = append(keep, e.Raw)
			seen = append(seen, e)
		}
	}
	return keep, removed
}

func pathClean(s *Shell, yes bool) int {
	if runtime.GOOS != "windows" {
		s.errorf("path nettoyer : seulement sous Windows")
		return 1
	}
	system, user := registryPath("system"), registryPath("user")
	type plan struct {
		scope   string
		keep    []string
		removed []string
	}
	var plans []plan
	if s.admin {
		k, r := cleanEntries(system, nil, dirExists)
		plans = append(plans, plan{"system", k, r})
	}
	// Une entrée utilisateur déjà présente dans le PATH système est un doublon.
	k, r := cleanEntries(user, system, dirExists)
	plans = append(plans, plan{"user", k, r})

	total := 0
	for _, p := range plans {
		if len(p.removed) == 0 {
			continue
		}
		label := map[string]string{"user": "PATH utilisateur", "system": "PATH système"}[p.scope]
		fmt.Fprintf(s.out, "%s : %d entrée(s) à retirer\n", label, len(p.removed))
		for _, r := range p.removed {
			fmt.Fprintf(s.out, "  - %s\n", r)
		}
		total += len(p.removed)
	}
	if total == 0 {
		fmt.Fprintln(s.out, "Rien à nettoyer : ni doublon ni dossier disparu.")
		if !s.admin {
			fmt.Fprintln(s.out, "(Le PATH système n'est vérifié qu'en administrateur : sudo path nettoyer)")
		}
		return 0
	}
	if !yes {
		fmt.Fprint(s.out, "\nLes retirer de Windows ? [o/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "o", "oui", "y", "yes":
		default:
			fmt.Fprintln(s.out, "Annulé.")
			return 1
		}
	}
	for _, p := range plans {
		if len(p.removed) == 0 {
			continue
		}
		if err := writePath(p.scope, p.keep); err != nil {
			s.errorf("path nettoyer : %v", err)
			return 1
		}
	}
	fmt.Fprintln(s.out, "PATH nettoyé. Les fenêtres ouvertes à partir de maintenant le voient.")
	if !s.admin {
		fmt.Fprintln(s.out, "Pour le PATH système aussi : sudo path nettoyer")
	}
	return 0
}
