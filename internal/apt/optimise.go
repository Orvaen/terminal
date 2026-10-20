package apt

// apt optimiser : accélère le développement sous Windows, en demandant avant chaque changement.
//
//   - Windows Defender n'analyse plus les caches des outils (npm, pip, Go, cargo…) ni le dossier des
//     projets : un npm install ou une compilation touche des dizaines de milliers de fichiers, chacun
//     analysé à l'ouverture.
//   - Docker reste démarré 1 h sans activité, au lieu de 10 min : moins de démarrages à froid.
//   - cmake --build (et make, si on le veut) utilisent tous les cœurs.
//   - sccache garde les compilations C/C++ et Rust en cache.
//
// Tout ce qui est changé est noté (optimisations.json) : « apt optimiser annuler » remet tout comme avant.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Orvaen/terminal/internal/docker"
	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Points d'injection, remplacés dans les tests.
var (
	optStatePath = func() string {
		dir, err := os.UserConfigDir()
		if err != nil {
			dir = os.TempDir()
		}
		return filepath.Join(dir, "OrvaenTerminal", "optimisations.json")
	}
	userEnv        = platform.UserEnv
	setUserEnv     = platform.SetUserEnv
	runPowerShell  = platform.PowerShell
	isElevated     = platform.IsElevated
	numCPU         = runtime.NumCPU
	defenderActive = func() bool {
		out, err := runPowerShell("(Get-MpComputerStatus).RealTimeProtectionEnabled")
		return err == nil && strings.TrimSpace(out) == "True"
	}
	askLine = func(question string) string {
		fmt.Fprint(stdout, question)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimSpace(line)
	}
	installSccache = func() int { return cmdInstall([]string{"Mozilla.sccache"}, true) }
)

// optState : ce que apt optimiser a changé, pour pouvoir l'annuler.
type optState struct {
	Defender []string           `json:"defender,omitempty"` // dossiers exclus de l'analyse
	Env      map[string]*string `json:"env,omitempty"`      // variable → valeur d'avant (nil : absente)
}

func loadOptState() optState {
	var st optState
	if b, err := os.ReadFile(optStatePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Env == nil {
		st.Env = map[string]*string{}
	}
	return st
}

func saveOptState(st optState) error {
	p := optStatePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// envOpt : un réglage fait par une variable d'environnement de l'utilisateur.
type envOpt struct {
	title, why string
	vars       [][2]string // nom, valeur
	defaultNo  bool        // réglage qui peut gêner : il faut répondre oui
	install    func() int  // à installer avant (sccache)
	needs      string      // programme requis par les variables
}

func (o envOpt) done() bool {
	for _, kv := range o.vars {
		if v, ok := userEnv(kv[0]); !ok || v != kv[1] {
			return false
		}
	}
	return true
}

func found(names ...string) bool {
	for _, n := range names {
		if _, err := toolLookPath(n); err == nil {
			return true
		}
	}
	return false
}

// envOpts : les réglages proposés sur ce PC (selon les outils installés).
func envOpts() []envOpt {
	n := strconv.Itoa(numCPU())
	var opts []envOpt
	if dockerInstalled() {
		opts = append(opts, envOpt{
			title: "Docker reste démarré 1 h sans activité (au lieu de 10 min)",
			why: "Moins de démarrages à froid (2 à 5 s à la première commande docker). En échange, la " +
				"machine Docker garde sa mémoire plus longtemps (docker engine stop la libère tout de suite).",
			vars: [][2]string{{"ORVAEN_DOCKER_IDLE", "3600"}},
		})
	}
	if found("cmake") {
		opts = append(opts, envOpt{
			title: "cmake --build sur tes " + n + " cœurs",
			why:   "Par défaut, cmake --build compile un fichier à la fois avec certains générateurs.",
			vars:  [][2]string{{"CMAKE_BUILD_PARALLEL_LEVEL", n}},
		})
	}
	if found("make", "mingw32-make") {
		opts = append(opts, envOpt{
			title: "make lance " + n + " tâches à la fois (MAKEFLAGS=-j" + n + ")",
			why: "Plus rapide sur les gros projets, mais certains Makefile ne le supportent pas (étapes qui " +
				"dépendent l'une de l'autre sans le dire) : à éviter si tes make enchaînent docker, tests… ",
			vars:      [][2]string{{"MAKEFLAGS", "-j" + n}},
			defaultNo: true,
		})
	}
	rust, cpp := found("cargo"), found("cl", "g++", "clang++", "gcc") || msvcCompilerInstalled()
	if rust || cpp {
		var vars [][2]string
		var langs []string
		if cpp {
			vars = append(vars, [2]string{"CMAKE_C_COMPILER_LAUNCHER", "sccache"}, [2]string{"CMAKE_CXX_COMPILER_LAUNCHER", "sccache"})
			langs = append(langs, "C/C++ (avec CMake)")
		}
		if rust {
			vars = append(vars, [2]string{"RUSTC_WRAPPER", "sccache"})
			langs = append(langs, "Rust")
		}
		o := envOpt{
			title: "sccache : cache de compilation pour " + strings.Join(langs, " et "),
			why:   "Un fichier déjà compilé avec les mêmes options n'est pas recompilé : les reconstructions complètes deviennent bien plus rapides.",
			vars:  vars,
			needs: "sccache",
		}
		if !found("sccache") {
			o.install = installSccache
		}
		opts = append(opts, o)
	}
	return opts
}

// msvcCompilerInstalled : le compilateur de Visual Studio, même hors du PATH.
var msvcCompilerInstalled = func() bool { return runtime.GOOS == "windows" && msvcInstalled() }

// defenderPaths : les caches et dossiers d'outils qui existent sur ce PC.
func defenderPaths() []string {
	local, roaming, home := os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA"), os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	var cands []string
	if local != "" {
		for _, d := range []string{"npm-cache", "pnpm", "pnpm-cache", "Yarn", `pip\Cache`, "pypoetry", "go-build",
			`Mozilla\sccache`, "OrvaenTerminal", "uv"} {
			cands = append(cands, filepath.Join(local, d))
		}
	}
	if roaming != "" {
		cands = append(cands, filepath.Join(roaming, "npm"), filepath.Join(roaming, "Composer"))
	}
	if home != "" {
		for _, d := range []string{"go", ".cargo", ".rustup", ".gradle", ".m2", `.nuget\packages`, ".bun", ".deno"} {
			cands = append(cands, filepath.Join(home, d))
		}
	}
	var out []string
	for _, c := range cands {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

// projectRoot : le dossier du dépôt Git (ou du projet) où l'on se trouve, "" sinon.
func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for d := dir; ; d = filepath.Dir(d) {
		for _, marker := range []string{".git", "package.json", "go.mod", "Cargo.toml", "pyproject.toml", "CMakeLists.txt"} {
			if _, err := os.Stat(filepath.Join(d, marker)); err == nil {
				return d
			}
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

func cmdOptimise(args []string, yes bool) int {
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	switch sub {
	case "--defender-ajouter", "--defender-retirer": // usage interne, dans la fenêtre administrateur
		return defenderApply(args[1:], sub == "--defender-ajouter")
	case "etat", "état", "status":
		optStatus()
		return 0
	case "annuler", "undo":
		return optUndo()
	case "":
	default:
		ui.Errorf("usage : apt optimiser [etat | annuler] [-y]")
		return 2
	}

	st := loadOptState()
	ui.Step("Optimisation de ce PC pour le développement")
	fmt.Fprintln(stdout, "Chaque changement est expliqué et demandé ; « apt optimiser annuler » remet tout comme avant.")
	ask := func(question string, defaultNo bool) bool {
		if yes {
			return !defaultNo
		}
		if !interactive() {
			return false
		}
		if defaultNo {
			fmt.Fprint(stdout, question+" [o/N] ")
			a := strings.ToLower(askLine(""))
			return a == "o" || a == "oui" || a == "y" || a == "yes"
		}
		return confirm(question)
	}
	code := 0

	// 1. Windows Defender
	fmt.Fprintln(stdout)
	if c := optimiseDefender(&st, ask, yes); c != 0 {
		code = c
	}

	// 2. Variables (Docker, compilation)
	for _, o := range envOpts() {
		fmt.Fprintln(stdout)
		if o.done() {
			fmt.Fprintln(stdout, ui.Color("32", "✓")+" "+o.title+" (déjà fait)")
			continue
		}
		fmt.Fprintln(stdout, ui.Color("1", "• "+o.title))
		fmt.Fprintln(stdout, "  "+o.why)
		var names []string
		for _, kv := range o.vars {
			names = append(names, kv[0]+"="+kv[1])
		}
		fmt.Fprintln(stdout, ui.Color("90", "  variables : "+strings.Join(names, "  ")))
		if !ask("  Appliquer ?", o.defaultNo) {
			fmt.Fprintln(stdout, "  passé.")
			continue
		}
		if o.install != nil {
			if c := o.install(); c != 0 {
				ui.Errorf("installation de %s impossible : réglage passé", o.needs)
				code = c
				continue
			}
		}
		if err := applyEnvOpt(&st, o); err != nil {
			ui.Errorf("%v", err)
			code = 1
			continue
		}
		fmt.Fprintln(stdout, "  "+ui.Color("32", "✓")+" fait (pour les fenêtres et programmes ouverts à partir de maintenant).")
	}

	if err := saveOptState(st); err != nil {
		ui.Warn("impossible d'enregistrer les changements (annulation impossible) : " + err.Error())
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, ui.Color("1", "À faire toi-même, si tu veux aller plus loin :"))
	fmt.Fprintln(stdout, "  • Dev Drive (Windows 11) : un disque fait pour le code, analysé plus légèrement.")
	fmt.Fprintln(stdout, "    Paramètres → Système → Stockage → Disques et volumes → Créer un Dev Drive.")
	fmt.Fprintln(stdout, "  • Docker : un dossier Windows monté dans un conteneur (-v C:\\…) est lent pour les")
	fmt.Fprintln(stdout, "    petits fichiers ; mets node_modules, .venv ou target dans un volume Docker.")
	return code
}

func applyEnvOpt(st *optState, o envOpt) error {
	for _, kv := range o.vars {
		if _, recorded := st.Env[kv[0]]; !recorded {
			if prev, ok := userEnv(kv[0]); ok {
				p := prev
				st.Env[kv[0]] = &p
			} else {
				st.Env[kv[0]] = nil
			}
		}
		if err := setUserEnv(kv[0], kv[1], false); err != nil {
			return fmt.Errorf("%s : %v", kv[0], err)
		}
		os.Setenv(kv[0], kv[1])
	}
	return nil
}

func optimiseDefender(st *optState, ask func(string, bool) bool, yes bool) int {
	if !defenderActive() {
		fmt.Fprintln(stdout, ui.Color("90", "– Windows Defender n'analyse pas en temps réel sur ce PC (autre antivirus ?) : rien à faire."))
		return 0
	}
	paths := defenderPaths()
	if root := projectRoot(); root != "" || !yes {
		q := "  Dossier de tes projets à exclure aussi"
		if root != "" {
			q += " [" + root + "]"
		}
		q += " (Entrée = proposé, un autre chemin, ou « non ») : "
		answer := ""
		if !yes && interactive() {
			answer = askLine(q)
		}
		switch strings.ToLower(answer) {
		case "":
			if root != "" {
				paths = append(paths, root)
			}
		case "non", "n", "no":
		default:
			if abs, err := filepath.Abs(answer); err == nil {
				if info, err := os.Stat(abs); err == nil && info.IsDir() {
					paths = append(paths, abs)
				} else {
					ui.Warn("dossier introuvable, ignoré : " + abs)
				}
			}
		}
	}
	already := map[string]bool{}
	for _, p := range st.Defender {
		already[strings.ToLower(p)] = true
	}
	var todo []string
	for _, p := range paths {
		if !already[strings.ToLower(p)] {
			todo = append(todo, p)
			already[strings.ToLower(p)] = true
		}
	}
	title := "Windows Defender n'analyse plus les caches des outils ni tes projets"
	if len(todo) == 0 {
		fmt.Fprintln(stdout, ui.Color("32", "✓")+" "+title+" (déjà fait)")
		return 0
	}
	fmt.Fprintln(stdout, ui.Color("1", "• "+title))
	fmt.Fprintln(stdout, "  npm install, pip, go build, cargo… ouvrent des milliers de petits fichiers, chacun analysé :")
	fmt.Fprintln(stdout, "  souvent 1,5 à 3 fois plus rapide sans. Ces dossiers ne seront plus analysés à l'ouverture,")
	fmt.Fprintln(stdout, "  donc n'y mets que du code en qui tu as confiance. Administrateur requis (fenêtre UAC).")
	for _, p := range todo {
		fmt.Fprintln(stdout, ui.Color("90", "    "+p))
	}
	if !ask("  Appliquer ?", false) {
		fmt.Fprintln(stdout, "  passé.")
		return 0
	}
	var code int
	if isElevated() {
		code = defenderApply(todo, true)
	} else {
		code = Elevate(append([]string{"optimiser", "--defender-ajouter"}, todo...)...)
	}
	if code != 0 {
		ui.Errorf("les exclusions n'ont pas été ajoutées (code %d)", code)
		return code
	}
	st.Defender = append(st.Defender, todo...)
	fmt.Fprintln(stdout, "  "+ui.Color("32", "✓")+" fait.")
	return 0
}

// defenderApply ajoute ou retire des exclusions de Windows Defender (en administrateur).
func defenderApply(paths []string, add bool) int {
	if len(paths) == 0 {
		return 0
	}
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = platform.PSQuote(p)
	}
	verb := "Add-MpPreference"
	if !add {
		verb = "Remove-MpPreference"
	}
	if _, err := runPowerShell("$ErrorActionPreference = 'Stop'; " + verb + " -ExclusionPath " + strings.Join(quoted, ",")); err != nil {
		ui.Errorf("Windows Defender : %v", err)
		return 1
	}
	return 0
}

func optStatus() {
	st := loadOptState()
	ui.Step("Optimisations de ce PC")
	if len(st.Defender) > 0 {
		fmt.Fprintf(stdout, "%s Windows Defender : %d dossiers exclus de l'analyse\n", ui.Color("32", "✓"), len(st.Defender))
		for _, p := range st.Defender {
			fmt.Fprintln(stdout, ui.Color("90", "    "+p))
		}
	} else {
		fmt.Fprintln(stdout, "○ Windows Defender : rien d'exclu par apt optimiser")
	}
	for _, o := range envOpts() {
		mark := "○"
		if o.done() {
			mark = ui.Color("32", "✓")
		}
		fmt.Fprintln(stdout, mark+" "+o.title)
	}
	fmt.Fprintf(stdout, "\nDocker : arrêt automatique après %s d'inactivité.\n", docker.IdleText())
	fmt.Fprintln(stdout, "Pour appliquer ce qui manque : apt optimiser ; pour tout annuler : apt optimiser annuler")
}

func optUndo() int {
	st := loadOptState()
	if len(st.Defender) == 0 && len(st.Env) == 0 {
		ui.Info("Rien à annuler : apt optimiser n'a rien changé sur ce PC.")
		return 0
	}
	ui.Step("Annulation des optimisations")
	code := 0
	if len(st.Defender) > 0 {
		var c int
		if isElevated() {
			c = defenderApply(st.Defender, false)
		} else {
			c = Elevate(append([]string{"optimiser", "--defender-retirer"}, st.Defender...)...)
		}
		if c == 0 {
			fmt.Fprintf(stdout, "%s Windows Defender analyse de nouveau %d dossiers\n", ui.Color("32", "✓"), len(st.Defender))
			st.Defender = nil
		} else {
			ui.Errorf("les exclusions de Windows Defender n'ont pas été retirées (code %d)", c)
			code = c
		}
	}
	names := make([]string, 0, len(st.Env))
	for k := range st.Env {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		prev := st.Env[k]
		var err error
		if prev == nil {
			err = setUserEnv(k, "", true)
			os.Unsetenv(k)
		} else {
			err = setUserEnv(k, *prev, false)
			os.Setenv(k, *prev)
		}
		if err != nil {
			ui.Errorf("%s : %v", k, err)
			code = 1
			continue
		}
		delete(st.Env, k)
		fmt.Fprintf(stdout, "%s %s remis comme avant\n", ui.Color("32", "✓"), k)
	}
	if err := saveOptState(st); err != nil {
		ui.Warn(err.Error())
	}
	return code
}
