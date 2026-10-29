package scripts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportCommand : ce que package.json lance pour un script de terminal.json.
// npm colle les arguments de « npm run add -- a b » à la fin : « terminal run add a b ».
func ExportCommand(name string) string { return "terminal run " + name }

// ExportResult : ce que l'export a fait.
type ExportResult struct {
	Added    []string // scripts ajoutés à package.json
	Replaced []string // scripts de package.json remplacés (--force)
	Kept     []string // déjà dans package.json, laissés tels quels
	Loops    []string // non exportés : ils appellent eux-mêmes « npm run <nom> »
	Created  bool     // package.json n'existait pas
}

// Export ajoute les scripts de terminal.json au package.json de son dossier.
func (r *Runner) Export(f *File, force bool) int {
	path := filepath.Join(f.Dir, "package.json")
	res, err := ExportTo(f, path, force)
	if err != nil {
		r.errorf("%v", err)
		return 1
	}
	list := func(names []string) string { return strings.Join(names, ", ") }
	switch {
	case len(res.Added)+len(res.Replaced) == 0:
		fmt.Fprintln(r.Out, "Rien à ajouter : package.json a déjà tous les scripts.")
	case res.Created:
		fmt.Fprintf(r.Out, "package.json créé avec %d script(s) : %s\n", len(res.Added), list(res.Added))
	default:
		if len(res.Added) > 0 {
			fmt.Fprintf(r.Out, "Ajouté(s) à package.json : %s\n", list(res.Added))
		}
		if len(res.Replaced) > 0 {
			fmt.Fprintf(r.Out, "Remplacé(s) : %s\n", list(res.Replaced))
		}
	}
	if len(res.Kept) > 0 {
		fmt.Fprintf(r.Out, "Déjà dans package.json, gardé(s) : %s  (run --export --force pour les remplacer)\n", list(res.Kept))
	}
	if len(res.Loops) > 0 {
		fmt.Fprintf(r.Out, "Pas exporté(s) : %s — ils lancent déjà « npm run » du même nom, npm tournerait en boucle.\n", list(res.Loops))
	}
	if n := len(res.Added) + len(res.Replaced); n > 0 {
		example := append(append([]string{}, res.Added...), res.Replaced...)[0]
		fmt.Fprintf(r.Out, "Lance-les avec : npm run %s   (arguments : npm run %s -- a b)\n", example, example)
	}
	return 0
}

// ExportTo écrit les scripts de f dans le package.json path (créé s'il n'existe pas).
// Les scripts déjà présents sont gardés, sauf avec force. Le reste du fichier, l'ordre
// de ses clés et son indentation sont conservés.
func ExportTo(f *File, path string, force bool) (ExportResult, error) {
	var res ExportResult
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		res.Created = true
		data = []byte("{\n  \"private\": true\n}\n")
	case err != nil:
		return res, err
	}

	doc, err := readOrdered(data)
	if err != nil {
		return res, fmt.Errorf("%s : %w", path, err)
	}
	var scripts []field
	if raw, ok := doc.get("scripts"); ok {
		o, err := readOrdered(raw)
		if err != nil {
			return res, fmt.Errorf(`%s : "scripts" doit être un objet`, path)
		}
		scripts = o.fields
	}
	existing := func(name string) int {
		for i, s := range scripts {
			if s.key == name {
				return i
			}
		}
		return -1
	}

	for _, s := range f.Scripts {
		if f.callsNpm(s.Name, s.Name, map[string]bool{}) {
			res.Loops = append(res.Loops, s.Name)
			continue
		}
		cmd, _ := json.Marshal(ExportCommand(s.Name))
		i := existing(s.Name)
		switch {
		case i < 0:
			scripts = append(scripts, field{s.Name, cmd})
			res.Added = append(res.Added, s.Name)
		case string(bytes.TrimSpace(scripts[i].raw)) == string(cmd):
			// déjà exporté
		case force:
			scripts[i].raw = cmd
			res.Replaced = append(res.Replaced, s.Name)
		default:
			res.Kept = append(res.Kept, s.Name)
		}
	}
	if len(res.Added)+len(res.Replaced) == 0 {
		return res, nil
	}

	doc.set("scripts", ordered{fields: scripts}.encode(doc.indent, doc.indent))
	out := doc.encode("", doc.indent)
	if doc.crlf {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	if doc.bom {
		out = append([]byte("\xef\xbb\xbf"), out...)
	}
	return res, os.WriteFile(path, out, 0o644)
}

// callsNpm dit si le script name (ou un script qu'il lance avec « run x ») lance « npm run target ».
func (f *File) callsNpm(name, target string, seen map[string]bool) bool {
	if seen[name] {
		return false
	}
	seen[name] = true
	s, ok := f.Get(name)
	if !ok {
		return false
	}
	for _, line := range append(append([]string{}, s.Steps...), s.Parallel...) {
		for _, cmd := range splitCommands(line) {
			w := strings.Fields(cmd)
			if len(w) == 0 {
				continue
			}
			if w[0] == "run" && len(w) > 1 {
				for _, n := range w[1:] {
					if !strings.HasPrefix(n, "-") && f.callsNpm(n, target, seen) {
						return true
					}
				}
			}
			if runsPackageScript(w, target) {
				return true
			}
		}
	}
	return false
}

// runsPackageScript : « npm run x », « npm run-script x », « npm start » (pour start, test,
// stop, restart), et pareil avec pnpm, yarn et bun.
func runsPackageScript(w []string, target string) bool {
	switch strings.TrimSuffix(strings.ToLower(w[0]), ".cmd") {
	case "npm", "pnpm", "yarn", "bun":
	default:
		return false
	}
	if len(w) < 2 {
		return false
	}
	if (w[1] == "run" || w[1] == "run-script") && len(w) > 2 {
		return w[2] == target
	}
	switch w[1] {
	case "start", "test", "stop", "restart":
		return w[1] == target
	}
	// yarn dev, pnpm dev, bun dev : le script directement.
	return w[0] != "npm" && w[1] == target
}

// splitCommands coupe une ligne sur &&, || et ; (approximation suffisante pour repérer npm run).
func splitCommands(line string) []string {
	r := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n")
	return strings.Split(r.Replace(line), "\n")
}

// --- objet JSON qui garde l'ordre des clés ---

type field struct {
	key string
	raw json.RawMessage
}

type ordered struct {
	fields []field
	indent string
	crlf   bool
	bom    bool
}

func readOrdered(data []byte) (ordered, error) {
	o := ordered{indent: "  "}
	if b, ok := bytes.CutPrefix(data, []byte("\xef\xbb\xbf")); ok {
		data, o.bom = b, true
	}
	o.crlf = bytes.Contains(data, []byte("\r\n"))
	o.indent = detectIndent(data)
	if firstByte(data) != '{' {
		return o, errors.New("attendu un objet JSON { … }")
	}
	err := orderedObject(data, func(k string, v json.RawMessage) error {
		o.fields = append(o.fields, field{k, append(json.RawMessage{}, v...)})
		return nil
	})
	if err != nil {
		return o, fmt.Errorf("JSON invalide : %w", err)
	}
	return o, nil
}

func (o *ordered) get(key string) (json.RawMessage, bool) {
	for _, f := range o.fields {
		if f.key == key {
			return f.raw, true
		}
	}
	return nil, false
}

func (o *ordered) set(key string, raw json.RawMessage) {
	for i := range o.fields {
		if o.fields[i].key == key {
			o.fields[i].raw = raw
			return
		}
	}
	o.fields = append(o.fields, field{key, raw})
}

// encode écrit l'objet, indenté de indent par niveau, à partir du niveau prefix.
func (o ordered) encode(prefix, indent string) []byte {
	var b bytes.Buffer
	if len(o.fields) == 0 {
		b.WriteString("{}")
	} else {
		b.WriteString("{\n")
		for i, f := range o.fields {
			key, _ := json.Marshal(f.key)
			b.WriteString(prefix + indent)
			b.Write(key)
			b.WriteString(": ")
			var compact, pretty bytes.Buffer
			if json.Compact(&compact, f.raw) == nil && json.Indent(&pretty, compact.Bytes(), prefix+indent, indent) == nil {
				b.Write(pretty.Bytes())
			} else {
				b.Write(bytes.TrimSpace(f.raw))
			}
			if i < len(o.fields)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(prefix + "}")
	}
	if prefix == "" {
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// detectIndent reprend l'indentation du fichier (tabulation ou espaces), 2 espaces par défaut.
func detectIndent(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || len(trimmed) == len(line) {
			continue
		}
		return line[:len(line)-len(trimmed)]
	}
	return "  "
}
