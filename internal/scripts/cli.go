package scripts

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Usage : aide de la commande run.
const Usage = `Usage : run [script] [-- arguments]

  run                    liste les scripts de terminal.json
  run <script>           lance un script
  run <script> -- args   ajoute des arguments à sa commande (run test -- f=00)
  run <a> <b> …          lance plusieurs scripts l'un après l'autre (arrêt au premier échec)
  run -p <a> <b> …       les lance en même temps, sorties préfixées (comme concurrently)
  run --init             crée terminal.json (avec les scripts de package.json et du Makefile)

terminal.json est cherché dans le dossier courant puis dans ses parents.
`

// Main exécute « run <args> » depuis le dossier cwd.
func Main(r *Runner, cwd string, args []string) int {
	var (
		parallel bool
		names    []string
		extra    []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			extra = args[i+1:]
			i = len(args)
		case a == "-h" || a == "--help":
			fmt.Fprint(r.Out, Usage)
			return 0
		case a == "--init":
			return r.Init(cwd)
		case a == "-p" || a == "--parallel":
			parallel = true
		case strings.HasPrefix(a, "-"):
			r.errorf("option inconnue : %s", a)
			fmt.Fprint(r.Err, Usage)
			return 2
		default:
			names = append(names, a)
		}
	}

	path, err := Find(cwd)
	if err != nil {
		r.errorf("%v", err)
		fmt.Fprintln(r.Err, "Pour en créer un : run --init")
		return 1
	}
	f, err := Load(path)
	if err != nil {
		r.errorf("%v", err)
		return 1
	}

	switch {
	case len(names) == 0:
		if len(extra) > 0 || parallel {
			r.errorf("précise au moins un script")
			return 2
		}
		r.List(f)
		return 0
	case len(extra) > 0 && len(names) > 1:
		r.errorf("les arguments après -- ne marchent qu'avec un seul script")
		return 2
	case parallel:
		return r.RunParallel(f, names)
	case len(names) > 1:
		return r.RunSequence(f, names)
	}
	return r.Run(f, names[0], extra)
}

// List affiche les scripts et ce qu'ils lancent.
func (r *Runner) List(f *File) {
	fmt.Fprintln(r.Out, "Scripts de "+f.Path+" :")
	width := 0
	for _, s := range f.Scripts {
		width = max(width, len(s.Name))
	}
	for _, s := range f.Scripts {
		what := strings.Join(s.Steps, " && ")
		if len(s.Parallel) > 0 {
			what = "en même temps : " + strings.Join(s.Parallel, " | ")
		}
		if s.Cwd != "" {
			what += r.paint("2", "  (dans "+s.Cwd+")")
		}
		fmt.Fprintf(r.Out, "  %s  %s\n", r.paint("1", fmt.Sprintf("%-*s", width, s.Name)), what)
		if s.Description != "" {
			fmt.Fprintf(r.Out, "  %-*s  %s\n", width, "", r.paint("2", s.Description))
		}
	}
	fmt.Fprintln(r.Out, "\nLance un script : run <nom>   Plusieurs en même temps : run -p <a> <b>")
}

// Init crée terminal.json dans dir, prérempli avec les scripts de package.json et les cibles du Makefile.
func (r *Runner) Init(dir string) int {
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); err == nil {
		r.errorf("%s existe déjà", path)
		return 1
	}
	type entry struct{ name, cmd string }
	var entries []entry
	seen := map[string]bool{}
	add := func(name, cmd string) {
		if !seen[name] {
			seen[name] = true
			entries = append(entries, entry{name, cmd})
		}
	}
	var sources []string
	for _, n := range makeTargets(filepath.Join(dir, "Makefile")) {
		add(n, "make "+n)
	}
	if len(entries) > 0 {
		sources = append(sources, "Makefile")
	}
	before := len(entries)
	for _, n := range npmScripts(filepath.Join(dir, "package.json")) {
		add(n, "npm run "+n)
	}
	if len(entries) > before {
		sources = append(sources, "package.json")
	}
	if len(entries) == 0 {
		add("hello", "echo Bonjour depuis terminal.json")
	}

	var b strings.Builder
	b.WriteString("{\n  \"scripts\": {\n")
	for i, e := range entries {
		name, _ := json.Marshal(e.name)
		cmd, _ := json.Marshal(e.cmd)
		fmt.Fprintf(&b, "    %s: %s", name, cmd)
		if i < len(entries)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }\n}\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		r.errorf("%v", err)
		return 1
	}
	msg := fmt.Sprintf("%s créé avec %d script(s)", FileName, len(entries))
	if len(sources) > 0 {
		msg += " repris de " + strings.Join(sources, " et ")
	}
	fmt.Fprintln(r.Out, msg+". Lance-les avec : run <nom>")
	return 0
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:([^=]|$)`)

// makeTargets lit les cibles d'un Makefile (hors cibles spéciales et règles à motif).
func makeTargets(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := makeTarget.FindStringSubmatch(sc.Text())
		if m == nil || seen[m[1]] || strings.Contains(m[1], ".") {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

// npmScripts lit les noms de « scripts » de package.json, dans l'ordre du fichier.
func npmScripts(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts json.RawMessage `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil || len(pkg.Scripts) == 0 {
		return nil
	}
	var out []string
	_ = orderedObject(pkg.Scripts, func(k string, _ json.RawMessage) error {
		if !strings.ContainsAny(k, " \t") {
			out = append(out, k)
		}
		return nil
	})
	return out
}
