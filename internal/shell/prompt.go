package shell

// Le prompt est décrit par un petit format, et chaque thème proposé n'est qu'un format prédéfini :
//
//	{nl}{vert}{user}@{host}{/} {admin} {jaune}{dir}{/}[[ {violet}({git}){/}]]{nl}{symbole}
//
// Éléments : {user} {host} {dir} (~/Desktop/projet) {dossier} (projet) {chemin} (C:\Users\…\projet)
// {git} (branche) {heure} {code} (code de sortie, si la commande a échoué) {admin} (ADMIN en
// administrateur) {symbole} ($ ou #, vert ou rouge) {nl} (retour à la ligne).
// Couleurs : {rouge} {vert} {jaune} {bleu} {violet} {cyan} {blanc} {gris} {gras} et {/} pour revenir
// à la normale ; fonds : {fond-bleu} {fond-gris} {fond-violet} {fond-vert} {fond-rouge}.
// [[ … ]] : partie affichée seulement si tous ses éléments ont une valeur (la branche hors d'un dépôt Git…).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Theme : un style de prompt proposé.
type Theme struct {
	Name, Description, Format string
}

// Themes : les styles proposés par « prompt ».
var Themes = []Theme{
	{"classique", "Deux lignes : utilisateur, PC et dossier, puis $",
		"{nl}{vert}{user}@{host}{/} {violet}{admin}{/} {jaune}{dir}{/}{nl}{symbole}"},
	{"minimal", "Le nom du dossier et une flèche",
		"{nl}{cyan}{dossier}{/} {symbole-fleche}"},
	{"chemin", "Le chemin complet et une flèche, sur une ligne",
		"{nl}{bleu}{gras}{dir}{/}[[ {violet}{git}{/}]] {symbole-fleche}"},
	{"git", "Comme bash dans Git Bash, avec la branche",
		"{nl}{vert}{user}@{host}{/} {jaune}{dir}{/}[[ {cyan}({git}){/}]]{nl}{symbole}"},
	{"fleche", "Deux lignes reliées, avec la branche et l'heure",
		"{nl}{gris}╭─{/} {vert}{user}{/} {gris}dans{/} {bleu}{gras}{dir}{/}[[ {gris}sur{/} {violet}{git}{/}]] {gris}à {heure}{/}[[ {rouge}✗ {code}{/}]]{nl}{gris}╰─{/}{symbole-fleche}"},
	{"blocs", "Des blocs de couleur (style powerline, sans police spéciale)",
		"{nl}{fond-bleu}{blanc}{gras} {dossier} {/}[[{fond-violet}{blanc} {git} {/}]][[{fond-rouge}{blanc} {code} {/}]] {symbole-fleche}"},
	{"windows", "Comme cmd : le chemin Windows suivi de >",
		"{nl}{chemin}>"},
	{"powershell", "Comme PowerShell : PS et le chemin Windows",
		"{nl}PS {chemin}> "},
}

// DefaultTheme : le thème par défaut.
const DefaultTheme = "classique"

func themeByName(name string) (Theme, bool) {
	for _, t := range Themes {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Theme{}, false
}

// ---------------------------------------------------------------------------
// Réglages (thème choisi, format personnalisé)

type promptConfig struct {
	Theme  string `json:"prompt"`
	Format string `json:"prompt_format,omitempty"`
}

// configPath : %APPDATA%\OrvaenTerminal\terminal-config.json (remplacé dans les tests).
var configPath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "OrvaenTerminal", "terminal-config.json")
}

func loadPromptConfig() promptConfig {
	var c promptConfig
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func savePromptConfig(c promptConfig) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Garder les autres réglages éventuels du fichier.
	all := map[string]any{}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	all["prompt"] = c.Theme
	if c.Format != "" {
		all["prompt_format"] = c.Format
	} else {
		delete(all, "prompt_format")
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// promptFormat : le format en vigueur (thème choisi, format perso, sinon classique).
func promptFormat(c promptConfig) string {
	if c.Theme == "perso" && c.Format != "" {
		return c.Format
	}
	if t, ok := themeByName(c.Theme); ok {
		return t.Format
	}
	t, _ := themeByName(DefaultTheme)
	return t.Format
}

// ---------------------------------------------------------------------------
// Rendu

// promptContext : ce que le prompt peut afficher.
type promptContext struct {
	User, Host, Cwd, Home, Git, Time string
	Code                             int
	Admin, Color                     bool
}

func (s *Shell) context() promptContext {
	cwd, _ := os.Getwd()
	host, _ := os.Hostname()
	return promptContext{
		User:  firstNonEmpty(os.Getenv("USERNAME"), os.Getenv("USER"), "user"),
		Host:  host,
		Cwd:   cwd,
		Home:  homeDir(),
		Git:   gitBranch(cwd),
		Time:  time.Now().Format("15:04"),
		Code:  s.last,
		Admin: s.admin,
		Color: s.color,
	}
}

var colorCodes = map[string]string{
	"rouge": "31", "vert": "32", "jaune": "33", "bleu": "34", "violet": "35", "cyan": "36", "blanc": "97",
	"gris": "90", "gras": "1", "fond-bleu": "44", "fond-gris": "100", "fond-violet": "45",
	"fond-vert": "42", "fond-rouge": "41",
}

var reToken = regexp.MustCompile(`\{([a-z/-]+)\}`)

// renderPrompt applique un format. Les éléments inconnus restent tels quels (pour voir l'erreur).
func renderPrompt(format string, c promptContext) string {
	var b strings.Builder
	for {
		start := strings.Index(format, "[[")
		if start < 0 {
			b.WriteString(renderPart(format, c))
			break
		}
		end := strings.Index(format[start:], "]]")
		if end < 0 {
			b.WriteString(renderPart(format, c))
			break
		}
		b.WriteString(renderPart(format[:start], c))
		group := format[start+2 : start+end]
		if groupFilled(group, c) {
			b.WriteString(renderPart(group, c))
		}
		format = format[start+end+2:]
	}
	return b.String()
}

// groupFilled : tous les éléments de valeur d'un groupe [[ … ]] sont non vides.
func groupFilled(group string, c promptContext) bool {
	for _, m := range reToken.FindAllStringSubmatch(group, -1) {
		if v, isValue := tokenValue(m[1], c); isValue && v == "" {
			return false
		}
	}
	return true
}

func renderPart(part string, c promptContext) string {
	return reToken.ReplaceAllStringFunc(part, func(tok string) string {
		name := tok[1 : len(tok)-1]
		if v, isValue := tokenValue(name, c); isValue {
			return v
		}
		if name == "/" {
			return ansi(c, "0")
		}
		if code, ok := colorCodes[name]; ok {
			return ansi(c, code)
		}
		return tok
	})
}

func ansi(c promptContext, code string) string {
	if !c.Color {
		return ""
	}
	return "\x1b[" + code + "m"
}

// tokenValue : la valeur d'un élément ; isValue=false pour une couleur ou un inconnu.
func tokenValue(name string, c promptContext) (string, bool) {
	symbolColor := "32"
	if c.Code != 0 {
		symbolColor = "31"
	}
	switch name {
	case "user":
		return c.User, true
	case "host":
		return c.Host, true
	case "dir":
		return displayPath(c.Cwd, c.Home), true
	case "dossier":
		base := filepath.Base(c.Cwd)
		if c.Home != "" && strings.EqualFold(filepath.Clean(c.Cwd), filepath.Clean(c.Home)) {
			base = "~"
		}
		return base, true
	case "chemin":
		return c.Cwd, true
	case "git":
		return c.Git, true
	case "heure":
		return c.Time, true
	case "code":
		if c.Code == 0 {
			return "", true
		}
		return itoa(c.Code), true
	case "admin":
		if c.Admin {
			return ansi(c, "1;31") + "ADMIN" + ansi(c, "0"), true
		}
		return "WIN", true
	case "symbole":
		sym := "$"
		if c.Admin {
			sym = "#"
		}
		return ansi(c, "1;"+symbolColor) + sym + ansi(c, "0") + " ", true
	case "signe": // \$ de bash : $, ou # en administrateur
		if c.Admin {
			return "#", true
		}
		return "$", true
	case "symbole-fleche":
		sym := "❯"
		if c.Admin {
			sym = "#"
		}
		return ansi(c, "1;"+symbolColor) + sym + ansi(c, "0") + " ", true
	case "nl":
		return "\n", true
	}
	return "", false
}

func itoa(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	b := []byte{}
	for {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

var reANSI = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visibleWidth : la largeur affichée d'un texte, sans les codes de couleur.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(reANSI.ReplaceAllString(s, ""))
}

// gitBranch lit la branche courante dans .git/HEAD (sans lancer git : le prompt reste instantané).
func gitBranch(dir string) string {
	for {
		gitPath := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if !info.IsDir() {
				// Sous-module ou worktree : « gitdir: chemin »
				b, err := os.ReadFile(gitPath)
				if err != nil {
					return ""
				}
				gd := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
				if !filepath.IsAbs(gd) {
					gd = filepath.Join(dir, gd)
				}
				gitPath = gd
			}
			head, err := os.ReadFile(filepath.Join(gitPath, "HEAD"))
			if err != nil {
				return ""
			}
			h := strings.TrimSpace(string(head))
			if ref, ok := strings.CutPrefix(h, "ref: refs/heads/"); ok {
				return ref
			}
			if len(h) >= 7 {
				return h[:7] // HEAD détachée : début du commit
			}
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
