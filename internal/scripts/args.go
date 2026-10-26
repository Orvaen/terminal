package scripts

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Arg : un argument déclaré dans « args » d'un script.
//
//	"args": ["user", "password", "role=admin"]
//
// « role=admin » donne une valeur par défaut ; sans valeur par défaut, l'argument est obligatoire.
type Arg struct {
	Name       string
	Default    string
	HasDefault bool
}

var argName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// parseArgs lit « args » : une liste de noms, « nom=défaut » pour une valeur par défaut.
func parseArgs(raw json.RawMessage) ([]Arg, error) {
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, errors.New(`"args" : attendu une liste de noms, ex. ["user", "password", "role=admin"]`)
	}
	var out []Arg
	seen := map[string]bool{}
	for _, item := range list {
		name, def, hasDef := strings.Cut(item, "=")
		name = strings.TrimSpace(name)
		if !argName.MatchString(name) {
			return nil, fmt.Errorf(`"args" : nom d'argument invalide %q (lettres, chiffres, _ et -)`, name)
		}
		if seen[name] {
			return nil, fmt.Errorf(`"args" : %q est déclaré deux fois`, name)
		}
		seen[name] = true
		out = append(out, Arg{Name: name, Default: def, HasDefault: hasDef})
	}
	return out, nil
}

// TakesArgs dit si un script attend des arguments : il en déclare dans « args », ou sa commande
// utilise $1, $@, ${nom}… Dans ce cas, tout ce qui suit son nom sur la ligne de run lui revient.
func (s Script) TakesArgs() bool {
	if len(s.Args) > 0 {
		return true
	}
	for _, l := range s.lines() {
		if hasPlaceholder(l, nil) {
			return true
		}
	}
	return false
}

// Usage : la forme d'appel d'un script, ex. « add <user> <password> [--role admin] ».
func (s Script) Usage() string {
	parts := []string{s.Name}
	for _, a := range s.Args {
		if a.HasDefault {
			parts = append(parts, fmt.Sprintf("[--%s %s]", a.Name, orEmpty(a.Default)))
		} else {
			parts = append(parts, "<"+a.Name+">")
		}
	}
	if len(s.Args) == 0 && s.TakesArgs() {
		parts = append(parts, "[arguments…]")
	}
	return strings.Join(parts, " ")
}

// Options : les options --nom du script (pour la complétion).
func (s Script) Options() []string {
	out := make([]string, len(s.Args))
	for i, a := range s.Args {
		out[i] = "--" + a.Name
	}
	return out
}

func orEmpty(s string) string {
	if s == "" {
		return `""`
	}
	return s
}

func (s Script) lines() []string {
	return append(append([]string{}, s.Steps...), s.Parallel...)
}

func (s Script) declared(name string) (Arg, bool) {
	for _, a := range s.Args {
		if a.Name == name {
			return a, true
		}
	}
	return Arg{}, false
}

// bind associe les arguments tapés aux arguments déclarés : --nom valeur, --nom=valeur,
// ou dans l'ordre pour ceux donnés sans nom.
func (s Script) bind(raw []string, strict bool) (map[string]string, error) {
	vals := map[string]string{}
	var positional []string
	for i := 0; i < len(raw); i++ {
		a := raw[i]
		if a == "--" {
			positional = append(positional, raw[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "--") || len(a) == 2 {
			positional = append(positional, a)
			continue
		}
		name, val, hasVal := strings.Cut(a[2:], "=")
		if _, ok := s.declared(name); !ok {
			if strict {
				return nil, fmt.Errorf("option inconnue : %s", a)
			}
			continue // gardée telle quelle dans $@ ou ajoutée à la commande
		}
		if !hasVal {
			if i+1 >= len(raw) {
				return nil, fmt.Errorf("--%s attend une valeur", name)
			}
			i++
			val = raw[i]
		}
		vals[name] = val
	}
	next := 0
	for _, a := range s.Args {
		if _, ok := vals[a.Name]; ok {
			continue
		}
		if next < len(positional) {
			vals[a.Name] = positional[next]
			next++
		}
	}
	if strict && len(s.Args) > 0 && next < len(positional) {
		return nil, fmt.Errorf("trop d'arguments : %s", strings.Join(positional[next:], " "))
	}
	var missing []string
	for _, a := range s.Args {
		if _, ok := vals[a.Name]; ok {
			continue
		}
		if a.HasDefault {
			vals[a.Name] = a.Default
		} else {
			missing = append(missing, a.Name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("argument manquant : %s", strings.Join(missing, ", "))
	}
	return vals, nil
}

// withArgs renvoie le script avec ses arguments appliqués :
//   - si ses commandes contiennent $1, $@, ${nom}…, ils sont remplacés ;
//   - sinon les arguments sont ajoutés à la fin de sa commande (comme npm run x -- args).
func (s Script) withArgs(raw []string) (Script, error) {
	hasPH, all := false, false
	for _, l := range s.lines() {
		if hasPlaceholder(l, s.Args) {
			hasPH = true
		}
		all = all || usesAll(l)
	}
	// Strict : une option ou un argument en trop serait perdu (pas de $@ ni d'ajout en fin de commande).
	vals, err := s.bind(raw, hasPH && !all)
	if err != nil {
		return s, err
	}
	if hasPH {
		sub := func(lines []string) []string {
			if lines == nil {
				return nil
			}
			out := make([]string, len(lines))
			for i, l := range lines {
				out[i] = substitute(l, vals, raw, s.Args)
			}
			return out
		}
		s.Steps, s.Parallel = sub(s.Steps), sub(s.Parallel)
		return s, nil
	}
	if len(raw) == 0 {
		return s, nil
	}
	if len(s.Steps) != 1 {
		return s, errors.New("pour passer des arguments à un script de plusieurs commandes, utilise $1, $@ ou ${nom} dans ses commandes")
	}
	s.Steps = []string{s.Steps[0] + " " + quoteArgs(stripDashDash(raw))}
	return s, nil
}

func stripDashDash(raw []string) []string {
	if len(raw) > 0 && raw[0] == "--" {
		return raw[1:]
	}
	return raw
}

// placeholder trouve un remplacement à la position i de line (qui commence par $).
// Renvoie le type (« all », « pos » ou « name »), la clé et la longueur.
func placeholder(line string, i int, args []Arg) (kind, key string, n int) {
	rest := line[i+1:]
	if rest == "" {
		return "", "", 0
	}
	switch c := rest[0]; {
	case c == '@' || c == '*':
		return "all", "", 2
	case c >= '1' && c <= '9':
		return "pos", rest[:1], 2
	case c == '{':
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", "", 0
		}
		key = rest[1:end]
		if key == "@" || key == "*" {
			return "all", "", end + 2
		}
		if isDigits(key) && key != "0" {
			return "pos", key, end + 2
		}
		if args == nil && argName.MatchString(key) {
			return "name?", key, end + 2
		}
		for _, a := range args {
			if a.Name == key {
				return "name", key, end + 2
			}
		}
	default:
		// $nom : seulement pour un argument déclaré (les autres restent des variables d'environnement).
		j := 0
		for j < len(rest) && (rest[j] == '_' || rest[j] >= 'a' && rest[j] <= 'z' || rest[j] >= 'A' && rest[j] <= 'Z' || j > 0 && rest[j] >= '0' && rest[j] <= '9') {
			j++
		}
		for _, a := range args {
			if j > 0 && a.Name == rest[:j] {
				return "name", a.Name, j + 1
			}
		}
	}
	return "", "", 0
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hasPlaceholder : la ligne contient $1, $@, $* ou ${nom} d'un argument déclaré.
// Avec args == nil, ${…} ne compte pas (c'est peut-être une variable d'environnement).
func hasPlaceholder(line string, args []Arg) bool {
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] == '$' {
			if kind, _, _ := placeholder(line, i, args); kind != "" && kind != "name?" {
				return true
			}
		}
	}
	return false
}

func usesAll(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] == '$' {
			if kind, _, _ := placeholder(line, i, nil); kind == "all" {
				return true
			}
		}
	}
	return false
}

// substitute remplace $1…, $@ et ${nom} dans line, en protégeant les valeurs
// selon l'endroit où elles tombent (hors guillemets, entre "…" ou entre '…').
func substitute(line string, vals map[string]string, raw []string, args []Arg) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\' && i+1 < len(line) && quote != '\'':
			b.WriteByte(c)
			b.WriteByte(line[i+1])
			i++
			continue
		case (c == '"' || c == '\'') && (quote == 0 || quote == c):
			if quote == 0 {
				quote = c
			} else {
				quote = 0
			}
		case c == '$':
			kind, key, n := placeholder(line, i, args)
			var words []string
			switch kind {
			case "all":
				words = stripDashDash(raw)
			case "pos":
				var idx int
				fmt.Sscan(key, &idx)
				if idx <= len(raw) {
					words = []string{raw[idx-1]}
				} else {
					words = []string{""}
				}
			case "name":
				words = []string{vals[key]}
			}
			if kind != "" && kind != "name?" {
				b.WriteString(quoteIn(quote, words))
				i += n - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// quoteIn écrit des valeurs pour qu'elles arrivent intactes, selon les guillemets ouverts.
func quoteIn(quote byte, words []string) string {
	switch quote {
	case '"':
		esc := make([]string, len(words))
		for i, w := range words {
			esc[i] = escapeDouble(w)
		}
		return strings.Join(esc, `" "`) // "$@" : un mot par argument
	case '\'':
		esc := make([]string, len(words))
		for i, w := range words {
			esc[i] = strings.ReplaceAll(w, "'", `'"'"'`)
		}
		return strings.Join(esc, `' '`)
	}
	return quoteArgs(words)
}

func escapeDouble(s string) string {
	r := strings.NewReplacer(`"`, `\"`, `$`, `\$`, "`", "\\`")
	return r.Replace(s)
}
