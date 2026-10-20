package shell

// Éditeur de ligne : saisie touche par touche, flèches, historique et complétion avec Tab.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

var errInterrupted = errors.New("interrompu")

// completer renvoie le début du mot à compléter (index en runes) et les mots candidats complets.
type completer func(line []rune, pos int) (start int, candidates []string)

type editor struct {
	in       *bufio.Reader
	out      io.Writer
	width    func() int
	complete completer
	onChange func(line string)
	history  []string

	row int // ligne (dans la saisie qui déborde) où se trouve le curseur
}

// readLine affiche prompt (dont la largeur visible est promptWidth) et lit une ligne.
// Renvoie io.EOF sur Ctrl+D ou Ctrl+Z avec une ligne vide, errInterrupted sur Ctrl+C.
func (e *editor) readLine(prompt string, promptWidth int) (string, error) {
	var (
		buf     []rune
		pos     int
		histIdx = len(e.history)
		draft   []rune
		lastTab bool
	)
	e.row = 0
	e.render(prompt, promptWidth, buf, pos)

	finish := func(suffix string) {
		e.render(prompt, promptWidth, buf, len(buf))
		fmt.Fprint(e.out, suffix+"\r\n")
		e.row = 0
	}

	for {
		r, _, err := e.in.ReadRune()
		if err != nil {
			if len(buf) == 0 {
				fmt.Fprint(e.out, "\r\n")
				return "", io.EOF
			}
			finish("")
			return string(buf), nil
		}
		tab := false
		switch r {
		case '\r', '\n':
			finish("")
			return string(buf), nil
		case 0x03: // Ctrl+C
			finish("^C")
			return "", errInterrupted
		case 0x04, 0x1a: // Ctrl+D, Ctrl+Z
			if len(buf) == 0 {
				fmt.Fprint(e.out, "\r\n")
				return "", io.EOF
			}
			if pos < len(buf) {
				buf = append(buf[:pos], buf[pos+1:]...)
			}
		case 0x7f, 0x08: // Retour arrière
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
			}
		case 0x01: // Ctrl+A
			pos = 0
		case 0x05: // Ctrl+E
			pos = len(buf)
		case 0x15: // Ctrl+U : efface avant le curseur
			buf = append([]rune(nil), buf[pos:]...)
			pos = 0
		case 0x0b: // Ctrl+K : efface après le curseur
			buf = buf[:pos]
		case 0x17: // Ctrl+W : efface le mot précédent
			start := wordLeft(buf, pos)
			buf = append(buf[:start], buf[pos:]...)
			pos = start
		case 0x0c: // Ctrl+L : efface l'écran
			fmt.Fprint(e.out, "\x1b[2J\x1b[H")
			e.row = 0
		case '\t':
			tab = true
			buf, pos = e.tab(prompt, promptWidth, buf, pos, lastTab)
		case 0x1b:
			var hist int
			buf, pos, hist = e.escape(buf, pos)
			switch {
			case hist < 0 && histIdx > 0: // ↑
				if histIdx == len(e.history) {
					draft = append([]rune(nil), buf...)
				}
				histIdx--
				buf = []rune(e.history[histIdx])
				pos = len(buf)
			case hist > 0 && histIdx < len(e.history): // ↓
				histIdx++
				if histIdx == len(e.history) {
					buf = append([]rune(nil), draft...)
				} else {
					buf = []rune(e.history[histIdx])
				}
				pos = len(buf)
			}
		default:
			if r >= 0x20 {
				buf = append(buf[:pos], append([]rune{r}, buf[pos:]...)...)
				pos++
			}
		}
		lastTab = tab
		if e.onChange != nil {
			e.onChange(string(buf))
		}
		e.render(prompt, promptWidth, buf, pos)
	}
}

// escape lit une séquence VT (flèches, Début, Fin, Suppr). hist vaut -1 pour ↑, +1 pour ↓.
func (e *editor) escape(buf []rune, pos int) ([]rune, int, int) {
	r, _, err := e.in.ReadRune()
	if err != nil {
		return buf, pos, 0
	}
	if r != '[' && r != 'O' {
		// Échap seul : efface la ligne (comme cmd), puis la touche suivante est traitée normalement
		_ = e.in.UnreadRune()
		return nil, 0, 0
	}
	var params strings.Builder
	for {
		r, _, err = e.in.ReadRune()
		if err != nil {
			return buf, pos, 0
		}
		if (r >= '0' && r <= '9') || r == ';' {
			params.WriteRune(r)
			continue
		}
		break
	}
	p := params.String()
	ctrl := strings.HasSuffix(p, ";5")
	switch r {
	case 'A':
		return buf, pos, -1
	case 'B':
		return buf, pos, 1
	case 'C':
		if ctrl {
			pos = wordRight(buf, pos)
		} else if pos < len(buf) {
			pos++
		}
	case 'D':
		if ctrl {
			pos = wordLeft(buf, pos)
		} else if pos > 0 {
			pos--
		}
	case 'H':
		pos = 0
	case 'F':
		pos = len(buf)
	case '~':
		switch p {
		case "1", "7":
			pos = 0
		case "4", "8":
			pos = len(buf)
		case "3": // Suppr
			if pos < len(buf) {
				buf = append(buf[:pos], buf[pos+1:]...)
			}
		}
	}
	return buf, pos, 0
}

func wordLeft(buf []rune, pos int) int {
	for pos > 0 && buf[pos-1] == ' ' {
		pos--
	}
	for pos > 0 && buf[pos-1] != ' ' {
		pos--
	}
	return pos
}

func wordRight(buf []rune, pos int) int {
	for pos < len(buf) && buf[pos] == ' ' {
		pos++
	}
	for pos < len(buf) && buf[pos] != ' ' {
		pos++
	}
	return pos
}

// tab complète le mot sous le curseur ; un second Tab affiche les possibilités.
func (e *editor) tab(prompt string, promptWidth int, buf []rune, pos int, again bool) ([]rune, int) {
	if e.complete == nil {
		return buf, pos
	}
	start, cands := e.complete(buf, pos)
	if len(cands) == 0 {
		fmt.Fprint(e.out, "\a")
		return buf, pos
	}
	word := strings.Trim(string(buf[start:pos]), `"`)

	var repl string
	switch {
	case len(cands) == 1:
		repl = quoteWord(cands[0], true)
		if !strings.HasSuffix(cands[0], "/") {
			repl += " "
		}
	default:
		common := commonPrefixFold(cands)
		if utf8.RuneCountInString(common) > utf8.RuneCountInString(word) {
			repl = quoteWord(common, false)
		} else {
			if again {
				e.list(prompt, promptWidth, buf, cands)
			} else {
				fmt.Fprint(e.out, "\a")
			}
			return buf, pos
		}
	}
	r := []rune(repl)
	out := append(append(append([]rune(nil), buf[:start]...), r...), buf[pos:]...)
	return out, start + len(r)
}

// quoteWord entoure de guillemets un mot qui contient une espace (fermés si le mot est complet).
func quoteWord(w string, complete bool) string {
	if !strings.ContainsAny(w, " \t") {
		return w
	}
	if complete && !strings.HasSuffix(w, "/") {
		return `"` + w + `"`
	}
	return `"` + w
}

func commonPrefixFold(words []string) string {
	first := []rune(words[0])
	n := len(first)
	for _, w := range words[1:] {
		rw := []rune(w)
		i := 0
		for i < n && i < len(rw) && strings.EqualFold(string(first[i]), string(rw[i])) {
			i++
		}
		n = i
	}
	return string(first[:n])
}

// list affiche les possibilités en colonnes sous la ligne, puis réaffiche la saisie.
func (e *editor) list(prompt string, promptWidth int, buf []rune, cands []string) {
	e.render(prompt, promptWidth, buf, len(buf))
	fmt.Fprint(e.out, "\r\n")
	const max = 200
	shown := cands
	if len(shown) > max {
		shown = shown[:max]
	}
	width := e.termWidth()
	colw := 0
	for _, c := range shown {
		if n := utf8.RuneCountInString(c); n > colw {
			colw = n
		}
	}
	colw += 2
	cols := width / colw
	if cols < 1 {
		cols = 1
	}
	rows := (len(shown) + cols - 1) / cols
	for r := 0; r < rows; r++ {
		var line strings.Builder
		for c := 0; c < cols; c++ {
			i := c*rows + r
			if i >= len(shown) {
				break
			}
			line.WriteString(shown[i])
			if (c+1)*rows+r < len(shown) {
				line.WriteString(strings.Repeat(" ", colw-utf8.RuneCountInString(shown[i])))
			}
		}
		fmt.Fprint(e.out, line.String()+"\r\n")
	}
	if len(cands) > max {
		fmt.Fprintf(e.out, "… et %d autres\r\n", len(cands)-max)
	}
	e.row = 0
}

func (e *editor) termWidth() int {
	if e.width != nil {
		if w := e.width(); w >= 10 {
			return w
		}
	}
	return 80
}

// render redessine la saisie, y compris quand elle déborde sur plusieurs lignes.
func (e *editor) render(prompt string, promptWidth int, buf []rune, pos int) {
	w := e.termWidth()
	var b strings.Builder
	if e.row > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", e.row)
	}
	b.WriteString("\r\x1b[J")
	b.WriteString(prompt)
	b.WriteString(string(buf))

	total := promptWidth + len(buf)
	endRow := total / w
	if total > 0 && total%w == 0 {
		b.WriteString("\n") // sort de l'état « retour à la ligne en attente » de la dernière colonne
	}
	target := promptWidth + pos
	tRow, tCol := target/w, target%w
	if endRow > tRow {
		fmt.Fprintf(&b, "\x1b[%dA", endRow-tRow)
	}
	b.WriteString("\r")
	if tCol > 0 {
		fmt.Fprintf(&b, "\x1b[%dC", tCol)
	}
	e.row = tRow
	fmt.Fprint(e.out, b.String())
}
