package shell

// Jokers et accolades, développés par le terminal comme par bash (sous Windows, les programmes
// reçoivent sinon « *.log » tel quel) :
//
//	*  ?  [abc] [a-z] [!abc]     dans un nom de fichier
//	**                          tous les sous-dossiers : src/**/*.ts
//	{a,b} {1..5} {a..e} {01..10} accolades : cp app.{ts,js} dist/
//
// Comme bash : rien entre guillemets n'est développé ; * ne prend pas les fichiers cachés (.env) sauf
// si le motif commence par un point ; un motif qui ne trouve rien reste tel quel. Sous Windows, la
// casse est ignorée, comme dans l'Explorateur.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// globCaseInsensitive : la casse compte-t-elle (remplacé dans les tests) ?
var globCaseInsensitive = runtime.GOOS == "windows"

// maxGlobVisits : au-delà, ** s'arrête (dossier gigantesque, node_modules…).
const maxGlobVisits = 200000

var errTooManyFiles = errors.New("trop de fichiers pour ce motif (plus de 200 000 parcourus) : précise le dossier")

// expandLine découpe une commande en arguments et développe ~, les accolades et les jokers.
func expandLine(text string) ([]string, error) {
	words, err := tokenizeWords(text, homeDir())
	if err != nil {
		return nil, err
	}
	return expandWords(words)
}

// expandWords applique les accolades puis les jokers à chaque mot.
func expandWords(words []word) ([]string, error) {
	var out []string
	for _, w := range words {
		for _, b := range expandBraces(w) {
			matches, err := globWord(b)
			if err != nil {
				return nil, err
			}
			if matches == nil {
				out = append(out, b.String())
			} else {
				out = append(out, matches...)
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Accolades

// expandBraces : « a{b,c}d » → « abd », « acd » ; « {1..3} » → 1 2 3. Les accolades entre guillemets,
// vides ou sans virgule ni « .. » restent telles quelles.
func expandBraces(w word) []word {
	open := -1
	depth := 0
	for i, r := range w.r {
		if w.q[i] {
			continue
		}
		switch r {
		case '{':
			if depth == 0 {
				open = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth != 0 {
				continue
			}
			inner := word{r: w.r[open+1 : i], q: w.q[open+1 : i]}
			alts := braceAlternatives(inner)
			if alts == nil {
				open = -1
				continue
			}
			prefix := word{r: w.r[:open], q: w.q[:open]}
			suffix := word{r: w.r[i+1:], q: w.q[i+1:]}
			var out []word
			for _, a := range alts {
				joined := concat(prefix, a, suffix)
				out = append(out, expandBraces(joined)...)
			}
			return out
		}
	}
	return []word{w}
}

func concat(parts ...word) word {
	var w word
	for _, p := range parts {
		w.r = append(w.r, p.r...)
		w.q = append(w.q, p.q...)
	}
	return w
}

// braceAlternatives : le contenu d'une paire d'accolades, découpé aux virgules de premier niveau,
// ou une suite {1..5}. nil si ce n'est pas une expansion (on garde les accolades).
func braceAlternatives(inner word) []word {
	var parts []word
	depth, start := 0, 0
	for i, r := range inner.r {
		if inner.q[i] {
			continue
		}
		switch r {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, word{r: inner.r[start:i], q: inner.q[start:i]})
				start = i + 1
			}
		}
	}
	if len(parts) > 0 {
		return append(parts, word{r: inner.r[start:], q: inner.q[start:]})
	}
	return braceSequence(inner)
}

// braceSequence : {1..5}, {05..10}, {10..1}, {a..e}, {1..10..2}.
func braceSequence(inner word) []word {
	for _, q := range inner.q {
		if q {
			return nil
		}
	}
	f := strings.Split(string(inner.r), "..")
	if len(f) != 2 && len(f) != 3 {
		return nil
	}
	step := 1
	if len(f) == 3 {
		n, err := strconv.Atoi(f[2])
		if err != nil || n == 0 {
			return nil
		}
		if n < 0 {
			n = -n
		}
		step = n
	}
	mk := func(s string) word { return word{r: []rune(s), q: make([]bool, len([]rune(s)))} }
	a, errA := strconv.Atoi(f[0])
	b, errB := strconv.Atoi(f[1])
	if errA == nil && errB == nil {
		width := 0
		if (len(f[0]) > 1 && strings.HasPrefix(strings.TrimPrefix(f[0], "-"), "0")) ||
			(len(f[1]) > 1 && strings.HasPrefix(strings.TrimPrefix(f[1], "-"), "0")) {
			width = max(len(f[0]), len(f[1]))
		}
		if abs(b-a)/step > 10000 {
			return nil
		}
		var out []word
		for i := a; ; {
			s := strconv.Itoa(i)
			if width > 0 && len(s) < width {
				s = strings.Repeat("0", width-len(s)) + s
			}
			out = append(out, mk(s))
			if i == b || (a < b && i+step > b) || (a > b && i-step < b) {
				break
			}
			if a < b {
				i += step
			} else {
				i -= step
			}
		}
		return out
	}
	ra, rb := []rune(f[0]), []rune(f[1])
	if len(ra) == 1 && len(rb) == 1 && unicode.IsLetter(ra[0]) && unicode.IsLetter(rb[0]) {
		var out []word
		for c := ra[0]; ; {
			out = append(out, mk(string(c)))
			if c == rb[0] || (ra[0] < rb[0] && c+rune(step) > rb[0]) || (ra[0] > rb[0] && c-rune(step) < rb[0]) {
				break
			}
			if ra[0] < rb[0] {
				c += rune(step)
			} else {
				c -= rune(step)
			}
		}
		return out
	}
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ---------------------------------------------------------------------------
// Jokers

// globWord renvoie les chemins qui correspondent au mot, triés ; nil s'il n'a pas de joker hors
// guillemets ou si rien ne correspond (le mot reste alors tel quel, comme dans bash).
func globWord(w word) ([]string, error) {
	if !w.hasUnquoted("*?[") {
		return nil, nil
	}
	// Découpage en segments aux / et \ (les deux séparent les dossiers sous Windows).
	type segment struct {
		w   word
		sep string // séparateur qui suit dans le texte d'origine
	}
	var segs []segment
	start := 0
	for i, r := range w.r {
		if r == '/' || (r == '\\' && runtime.GOOS == "windows") {
			segs = append(segs, segment{w: word{r: w.r[start:i], q: w.q[start:i]}, sep: string(r)})
			start = i + 1
		}
	}
	segs = append(segs, segment{w: word{r: w.r[start:], q: w.q[start:]}})

	// Point de départ : chemin absolu (/, C:\, /c/ façon Git Bash) ou dossier courant.
	walkRoot, shownRoot := ".", ""
	i := 0
	switch {
	case len(segs) > 1 && segs[0].w.String() == "":
		// « /… » : sous Windows, /c/… est le disque C:
		if runtime.GOOS == "windows" && len(segs) > 2 && len(segs[1].w.r) == 1 && unicode.IsLetter(segs[1].w.r[0]) && !segs[1].w.hasUnquoted("*?[") {
			walkRoot = strings.ToUpper(segs[1].w.String()) + `:\`
			shownRoot = "/" + segs[1].w.String() + "/"
			i = 2
		} else {
			walkRoot, shownRoot, i = string(filepath.Separator), segs[0].sep, 1
		}
	case len(segs) > 1 && runtime.GOOS == "windows" && len(segs[0].w.r) == 2 && segs[0].w.r[1] == ':':
		walkRoot = segs[0].w.String() + `\`
		shownRoot = segs[0].w.String() + segs[0].sep
		i = 1
	}

	type state struct{ fs, shown string }
	current := []state{{walkRoot, shownRoot}}
	visits := 0
	for ; i < len(segs); i++ {
		seg, last := segs[i], i == len(segs)-1
		sep := seg.sep
		if sep == "" {
			sep = "/"
		}
		var next []state
		text := seg.w.String()
		switch {
		case text == "" || text == ".":
			for _, c := range current {
				next = append(next, state{c.fs, c.shown + text + seg.sep})
			}
		case text == "**" && !seg.w.q[0] && !seg.w.q[1]:
			// zéro ou plusieurs sous-dossiers (sans les dossiers cachés)
			for _, c := range current {
				next = append(next, c)
				err := walkDirs(c.fs, c.shown, sep, &visits, func(fs, shown string) {
					next = append(next, state{fs, shown})
				})
				if err != nil {
					return nil, err
				}
			}
		case !seg.w.hasUnquoted("*?["):
			for _, c := range current {
				p := filepath.Join(c.fs, text)
				if last {
					if _, err := os.Lstat(p); err != nil {
						continue
					}
				} else if !isDir(p) {
					continue
				}
				next = append(next, state{p, c.shown + text + seg.sep})
			}
		default:
			pat := compilePattern(seg.w)
			for _, c := range current {
				entries, err := os.ReadDir(c.fs)
				if err != nil {
					continue
				}
				for _, e := range entries {
					visits++
					if visits > maxGlobVisits {
						return nil, errTooManyFiles
					}
					name := e.Name()
					if strings.HasPrefix(name, ".") && !pat.dotOK {
						continue
					}
					if !pat.match(name) {
						continue
					}
					p := filepath.Join(c.fs, name)
					if !last && !isDir(p) {
						continue
					}
					next = append(next, state{p, c.shown + name + seg.sep})
				}
			}
		}
		current = next
		if len(current) == 0 {
			return nil, nil
		}
	}

	seen := map[string]bool{}
	var out []string
	for _, c := range current {
		shown := c.shown
		if shown == "" {
			continue
		}
		if !seen[shown] {
			seen[shown] = true
			out = append(out, shown)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if globCaseInsensitive {
			return strings.ToLower(out[a]) < strings.ToLower(out[b])
		}
		return out[a] < out[b]
	})
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// walkDirs appelle fn pour chaque sous-dossier (récursivement, sans les dossiers cachés).
func walkDirs(root, shown, sep string, visits *int, fn func(fs, shown string)) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		*visits++
		if *visits > maxGlobVisits {
			return errTooManyFiles
		}
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(root, e.Name())
		s := shown + e.Name() + sep
		fn(p, s)
		if err := walkDirs(p, s, sep, visits, fn); err != nil {
			return err
		}
	}
	return nil
}

// pattern : un segment de motif compilé (* ? [..] hors guillemets, le reste littéral).
type pattern struct {
	items []patItem
	dotOK bool // le motif commence par un point littéral : les fichiers cachés sont pris
}

type patItem struct {
	kind   byte // 'l' littéral, '*', '?', '['
	r      rune
	negate bool
	ranges [][2]rune
}

func compilePattern(w word) pattern {
	var p pattern
	p.dotOK = len(w.r) > 0 && w.r[0] == '.'
	for i := 0; i < len(w.r); i++ {
		r, q := w.r[i], w.q[i]
		switch {
		case q:
			p.items = append(p.items, patItem{kind: 'l', r: r})
		case r == '*':
			if len(p.items) == 0 || p.items[len(p.items)-1].kind != '*' {
				p.items = append(p.items, patItem{kind: '*'})
			}
		case r == '?':
			p.items = append(p.items, patItem{kind: '?'})
		case r == '[':
			if item, end, ok := parseClass(w, i); ok {
				p.items = append(p.items, item)
				i = end
			} else {
				p.items = append(p.items, patItem{kind: 'l', r: r})
			}
		default:
			p.items = append(p.items, patItem{kind: 'l', r: r})
		}
	}
	return p
}

// parseClass lit [abc], [a-z], [!abc] ou [^abc] à partir de w.r[i] == '['.
func parseClass(w word, i int) (patItem, int, bool) {
	item := patItem{kind: '['}
	j := i + 1
	if j < len(w.r) && (w.r[j] == '!' || w.r[j] == '^') {
		item.negate = true
		j++
	}
	first := true
	for ; j < len(w.r); j++ {
		r := w.r[j]
		if r == ']' && !first {
			return item, j, len(item.ranges) > 0
		}
		first = false
		if j+2 < len(w.r) && w.r[j+1] == '-' && w.r[j+2] != ']' {
			item.ranges = append(item.ranges, [2]rune{r, w.r[j+2]})
			j += 2
			continue
		}
		item.ranges = append(item.ranges, [2]rune{r, r})
	}
	return item, 0, false
}

func (p pattern) match(name string) bool {
	return matchItems(p.items, []rune(name))
}

func foldRune(r rune) rune {
	if globCaseInsensitive {
		return unicode.ToLower(r)
	}
	return r
}

func matchItems(items []patItem, s []rune) bool {
	// Correspondance avec retour arrière sur la dernière étoile (linéaire en pratique).
	pi, si := 0, 0
	starP, starS := -1, 0
	for si < len(s) {
		if pi < len(items) {
			it := items[pi]
			switch it.kind {
			case '*':
				starP, starS = pi, si
				pi++
				continue
			case '?':
				pi++
				si++
				continue
			case '[':
				if classMatch(it, s[si]) {
					pi++
					si++
					continue
				}
			default:
				if foldRune(it.r) == foldRune(s[si]) {
					pi++
					si++
					continue
				}
			}
		}
		if starP >= 0 {
			starS++
			si = starS
			pi = starP + 1
			continue
		}
		return false
	}
	for pi < len(items) && items[pi].kind == '*' {
		pi++
	}
	return pi == len(items)
}

func classMatch(it patItem, r rune) bool {
	r = foldRune(r)
	in := false
	for _, rg := range it.ranges {
		lo, hi := foldRune(rg[0]), foldRune(rg[1])
		if lo <= r && r <= hi {
			in = true
			break
		}
	}
	return in != it.negate
}
