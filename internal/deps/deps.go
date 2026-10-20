// Package deps détecte les outils dont un projet a besoin (Node, Go, Python, Docker, make…)
// à partir de ses fichiers, avec les versions demandées, pour « apt deps ».
package deps

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Tool décrit un outil que l'on sait reconnaître et installer.
type Tool struct {
	Key      string
	Label    string
	Commands []string // programmes qui prouvent qu'il est installé (le premier donne la version)
	Package  string   // nom pour « apt install » ; "" : ne s'installe pas avec apt (voir Note)
	Note     string
}

// catalog : ordre d'affichage.
var catalog = []Tool{
	{Key: "git", Label: "Git", Commands: []string{"git"}, Package: "git"},
	{Key: "docker", Label: "Docker", Commands: []string{"docker"}, Package: "docker"},
	{Key: "make", Label: "make", Commands: []string{"make"}, Package: "make"},
	{Key: "node", Label: "Node.js", Commands: []string{"node"}, Package: "node"},
	{Key: "pnpm", Label: "pnpm", Commands: []string{"pnpm"}, Package: "pnpm"},
	{Key: "yarn", Label: "Yarn", Commands: []string{"yarn"}, Package: "yarn"},
	{Key: "bun", Label: "Bun", Commands: []string{"bun"}, Package: "bun"},
	{Key: "deno", Label: "Deno", Commands: []string{"deno"}, Package: "deno"},
	{Key: "go", Label: "Go", Commands: []string{"go"}, Package: "go"},
	{Key: "rust", Label: "Rust", Commands: []string{"cargo", "rustc"}, Package: "rust"},
	{Key: "python", Label: "Python", Commands: []string{"python", "py"}, Package: "python"},
	{Key: "uv", Label: "uv", Commands: []string{"uv"}, Package: "uv"},
	{Key: "poetry", Label: "Poetry", Commands: []string{"poetry"},
		Note: "s'installe avec Python : python -m pip install --user poetry (ou uv tool install poetry)"},
	{Key: "java", Label: "Java (JDK)", Commands: []string{"java", "javac"}, Package: "java"},
	{Key: "maven", Label: "Maven", Commands: []string{"mvn"},
		Note: "pas dans winget : ajoute le wrapper au projet (mvn -N wrapper:wrapper) puis utilise ./mvnw"},
	{Key: "gradle", Label: "Gradle", Commands: []string{"gradle"},
		Note: "pas dans winget : utilise le wrapper du projet (gradlew), qui télécharge Gradle tout seul"},
	{Key: "dotnet", Label: ".NET SDK", Commands: []string{"dotnet"}, Package: "dotnet"},
	{Key: "php", Label: "PHP", Commands: []string{"php"}, Package: "php"},
	{Key: "composer", Label: "Composer", Commands: []string{"composer"},
		Note: "pas dans winget : installe-le avec PHP, voir https://getcomposer.org/download/"},
	{Key: "ruby", Label: "Ruby", Commands: []string{"ruby"}, Package: "ruby"},
	{Key: "cmake", Label: "CMake", Commands: []string{"cmake"}, Package: "cmake"},
	{Key: "cpp", Label: "Compilateur C/C++", Commands: []string{"cl", "gcc", "clang"},
		Note: "MSVC : apt install Microsoft.VisualStudio.2022.BuildTools, puis la charge de travail « Développement Desktop en C++ » ; ou gcc via apt install msys2"},
	{Key: "terraform", Label: "Terraform", Commands: []string{"terraform"}, Package: "terraform"},
	{Key: "helm", Label: "Helm", Commands: []string{"helm"}, Package: "helm"},
	{Key: "hurl", Label: "Hurl", Commands: []string{"hurl"}, Package: "hurl"},
	{Key: "elixir", Label: "Elixir", Commands: []string{"elixir", "mix"}, Package: "elixir"},
	{Key: "erlang", Label: "Erlang/OTP", Commands: []string{"erl"}, Package: "erlang"},
	{Key: "dart", Label: "Dart", Commands: []string{"dart"}, Package: "dart"},
	{Key: "swift", Label: "Swift", Commands: []string{"swift"}, Package: "swift"},
	{Key: "perl", Label: "Perl", Commands: []string{"perl"}, Package: "perl"},
	{Key: "haskell", Label: "Haskell Stack", Commands: []string{"stack"}, Package: "stack"},
	{Key: "sbt", Label: "sbt (Scala)", Commands: []string{"sbt"}, Package: "sbt"},
}

func toolByKey(key string) Tool {
	for _, t := range catalog {
		if t.Key == key {
			return t
		}
	}
	return Tool{Key: key, Label: key}
}

// Need : un outil demandé par le projet.
type Need struct {
	Tool
	Sources     []string // fichiers qui le demandent (relatifs au projet)
	Version     string   // version minimale demandée ("" : aucune)
	VersionFrom string   // fichier qui demande la version
}

// Step : une commande qui installe les dépendances du projet (npm ci, go mod download…).
type Step struct {
	Dir   string     // relatif au projet ("." pour la racine)
	Args  [][]string // commandes, l'une après l'autre
	Label string     // affichage
	Done  bool       // semble déjà fait (node_modules présent…)
}

// Project : résultat de l'analyse.
type Project struct {
	Root  string
	Needs []Need
	Steps []Step
}

// skipDirs : dossiers jamais parcourus.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true, "target": true, "dist": true, "build": true,
	".venv": true, "venv": true, "__pycache__": true, ".next": true, ".nuxt": true, "bin": true,
	"obj": true, ".idea": true, ".vscode": true, ".gradle": true, ".terraform": true, "out": true,
	"coverage": true, ".cache": true, ".turbo": true,
}

// maxDepth : profondeur parcourue (racine = 0), pour les projets en plusieurs dossiers (api/, web/…).
const maxDepth = 3

type detector struct {
	p     *Project
	needs map[string]*Need
}

func (d *detector) need(key, source string) *Need {
	n, ok := d.needs[key]
	if !ok {
		n = &Need{Tool: toolByKey(key)}
		d.needs[key] = n
	}
	for _, s := range n.Sources {
		if s == source {
			return n
		}
	}
	n.Sources = append(n.Sources, source)
	return n
}

// want enregistre une version minimale ; la plus exigeante l'emporte.
func (d *detector) want(key, version, source string) {
	v := cleanVersion(version)
	if v == "" {
		return
	}
	n := d.need(key, source)
	if n.Version == "" || compareVersions(v, n.Version) > 0 {
		n.Version, n.VersionFrom = v, source
	}
}

// Detect analyse le projet situé dans root.
func Detect(root string) (*Project, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	d := &detector{p: &Project{Root: root}, needs: map[string]*Need{}}
	if info, err := os.Stat(filepath.Join(root, ".git")); err == nil && info.IsDir() {
		d.need("git", ".git")
	}
	err = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if e.IsDir() {
			if path != root && (skipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".")) {
				return filepath.SkipDir
			}
			if strings.Count(rel, "/")+1 > maxDepth && path != root {
				return filepath.SkipDir
			}
			d.dir(path, rel)
			return nil
		}
		d.file(path, rel, e.Name())
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, t := range catalog {
		if n, ok := d.needs[t.Key]; ok {
			d.p.Needs = append(d.p.Needs, *n)
		}
	}
	d.p.Steps = dropNested(d.p.Steps)
	sort.SliceStable(d.p.Steps, func(i, j int) bool { return d.p.Steps[i].Dir < d.p.Steps[j].Dir })
	return d.p, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(string(b), "\xef\xbb\xbf")
}

func join(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// dir : ce qui dépend du contenu d'un dossier (gestionnaire de paquets JavaScript, Python…).
func (d *detector) dir(path, rel string) {
	if rel == "" {
		rel = "."
	}
	has := func(n string) bool { return exists(filepath.Join(path, n)) }

	if has("package.json") {
		args := []string{"npm", "install"}
		switch {
		case has("pnpm-lock.yaml"):
			args = []string{"pnpm", "install"}
			d.need("pnpm", join(rel, "pnpm-lock.yaml"))
		case has("yarn.lock"):
			args = []string{"yarn", "install"}
			d.need("yarn", join(rel, "yarn.lock"))
		case has("bun.lockb") || has("bun.lock"):
			args = []string{"bun", "install"}
			d.need("bun", join(rel, "bun.lock"))
		case has("package-lock.json"):
			args = []string{"npm", "ci"}
		}
		d.p.Steps = append(d.p.Steps, Step{Dir: rel, Args: [][]string{args}, Label: strings.Join(args, " "),
			Done: has("node_modules")})
	}

	pyReqs := has("requirements.txt")
	switch {
	case has("uv.lock"):
		d.need("uv", join(rel, "uv.lock"))
		d.p.Steps = append(d.p.Steps, Step{Dir: rel, Args: [][]string{{"uv", "sync"}}, Label: "uv sync", Done: has(".venv")})
	case has("poetry.lock"):
		d.need("poetry", join(rel, "poetry.lock"))
		d.p.Steps = append(d.p.Steps, Step{Dir: rel, Args: [][]string{{"poetry", "install"}}, Label: "poetry install"})
	case pyReqs:
		venvPy := filepath.Join(".venv", "Scripts", "python")
		if runtime.GOOS != "windows" {
			venvPy = filepath.Join(".venv", "bin", "python")
		}
		args := [][]string{{"python", "-m", "venv", ".venv"}, {venvPy, "-m", "pip", "install", "-r", "requirements.txt"}}
		label := "python -m venv .venv && " + filepath.ToSlash(venvPy) + " -m pip install -r requirements.txt"
		d.p.Steps = append(d.p.Steps, Step{Dir: rel, Args: args, Label: label, Done: has(".venv")})
	}
}

var (
	reGoDirective  = regexp.MustCompile(`(?m)^go\s+(\d+(?:\.\d+){0,2})\s*$`)
	reRustVersion  = regexp.MustCompile(`(?m)^rust-version\s*=\s*"([^"]+)"`)
	reRequiresPy   = regexp.MustCompile(`(?m)^requires-python\s*=\s*"([^"]+)"`)
	reToolUV       = regexp.MustCompile(`(?m)^\[tool\.uv`)
	reToolPoetry   = regexp.MustCompile(`(?m)^\[tool\.poetry`)
	reJavaPom      = regexp.MustCompile(`<(?:maven\.compiler\.release|maven\.compiler\.source|java\.version|release)>\s*(\d+(?:\.\d+)?)\s*<`)
	reJavaGradle   = regexp.MustCompile(`(?:JavaLanguageVersion\.of\(\s*|JavaVersion\.VERSION_|jvmToolchain\(\s*|sourceCompatibility\s*=\s*['"]?)(\d+)`)
	reRustChannel  = regexp.MustCompile(`(?m)^channel\s*=\s*"([^"]+)"`)
	reComposePHP   = regexp.MustCompile(`"php"\s*:\s*"([^"]+)"`)
	reMakeDocker   = regexp.MustCompile(`(?m)^\t.*\bdocker\b`)
	reVersionToken = regexp.MustCompile(`\d+(?:\.\d+){0,2}`)
)

// file : ce qu'un fichier révèle.
func (d *detector) file(path, rel, name string) {
	lower := strings.ToLower(name)
	switch {
	case name == "package.json":
		d.need("node", rel)
		var pkg struct {
			Engines        map[string]string `json:"engines"`
			PackageManager string            `json:"packageManager"`
			Volta          map[string]string `json:"volta"`
		}
		if json.Unmarshal([]byte(readFile(path)), &pkg) == nil {
			d.want("node", pkg.Engines["node"], rel)
			d.want("node", pkg.Volta["node"], rel)
			if pm, ver, ok := strings.Cut(pkg.PackageManager, "@"); ok || pm != "" {
				switch pm {
				case "pnpm", "yarn", "bun":
					d.need(pm, rel)
					if pm != "yarn" { // Yarn 2+ s'installe par projet (corepack) : la version n'est pas celle de l'outil global
						d.want(pm, ver, rel)
					}
				}
			}
		}
	case name == ".nvmrc" || name == ".node-version":
		d.need("node", rel)
		d.want("node", readFile(path), rel)
	case name == "deno.json" || name == "deno.jsonc":
		d.need("deno", rel)
	case name == "go.mod":
		d.need("go", rel)
		if m := reGoDirective.FindStringSubmatch(readFile(path)); m != nil {
			d.want("go", m[1], rel)
		}
		d.p.Steps = append(d.p.Steps, Step{Dir: dirOf(rel), Args: [][]string{{"go", "mod", "download"}}, Label: "go mod download"})
	case name == "Cargo.toml":
		d.need("rust", rel)
		if m := reRustVersion.FindStringSubmatch(readFile(path)); m != nil {
			d.want("rust", m[1], rel)
		}
		d.p.Steps = append(d.p.Steps, Step{Dir: dirOf(rel), Args: [][]string{{"cargo", "fetch"}}, Label: "cargo fetch"})
	case name == "rust-toolchain" || name == "rust-toolchain.toml":
		d.need("rust", rel)
		content := readFile(path)
		if m := reRustChannel.FindStringSubmatch(content); m != nil {
			content = m[1]
		}
		d.want("rust", content, rel)
	case name == "requirements.txt" || name == "setup.py" || name == "Pipfile" || name == "pyproject.toml" || name == "runtime.txt":
		d.need("python", rel)
		content := readFile(path)
		if name == "pyproject.toml" {
			if m := reRequiresPy.FindStringSubmatch(content); m != nil {
				d.want("python", m[1], rel)
			}
			if reToolUV.MatchString(content) {
				d.need("uv", rel)
			}
			if reToolPoetry.MatchString(content) {
				d.need("poetry", rel)
			}
		}
		if name == "runtime.txt" && strings.HasPrefix(strings.TrimSpace(content), "python-") {
			d.want("python", strings.TrimPrefix(strings.TrimSpace(content), "python-"), rel)
		}
	case name == ".python-version":
		d.need("python", rel)
		d.want("python", readFile(path), rel)
	case name == "Gemfile":
		d.need("ruby", rel)
		d.p.Steps = append(d.p.Steps, Step{Dir: dirOf(rel), Args: [][]string{{"bundle", "install"}}, Label: "bundle install"})
	case name == ".ruby-version":
		d.need("ruby", rel)
		d.want("ruby", readFile(path), rel)
	case name == "composer.json":
		d.need("php", rel)
		d.need("composer", rel)
		if m := reComposePHP.FindStringSubmatch(readFile(path)); m != nil {
			d.want("php", m[1], rel)
		}
		d.p.Steps = append(d.p.Steps, Step{Dir: dirOf(rel), Args: [][]string{{"composer", "install"}}, Label: "composer install",
			Done: exists(filepath.Join(filepath.Dir(path), "vendor"))})
	case name == "pom.xml":
		d.need("java", rel)
		if !exists(filepath.Join(filepath.Dir(path), "mvnw.cmd")) && !exists(filepath.Join(filepath.Dir(path), "mvnw")) {
			d.need("maven", rel)
		}
		if m := reJavaPom.FindStringSubmatch(readFile(path)); m != nil {
			d.want("java", m[1], rel)
		}
	case name == "build.gradle" || name == "build.gradle.kts" || name == "settings.gradle" || name == "settings.gradle.kts":
		d.need("java", rel)
		if !exists(filepath.Join(filepath.Dir(path), "gradlew.bat")) && !exists(filepath.Join(filepath.Dir(path), "gradlew")) {
			d.need("gradle", rel)
		}
		if m := reJavaGradle.FindStringSubmatch(readFile(path)); m != nil {
			d.want("java", m[1], rel)
		}
	case strings.HasSuffix(lower, ".csproj") || strings.HasSuffix(lower, ".fsproj") || strings.HasSuffix(lower, ".vbproj") || strings.HasSuffix(lower, ".sln") || strings.HasSuffix(lower, ".slnx"):
		d.need("dotnet", rel)
		d.addStepOnce(Step{Dir: dirOf(rel), Args: [][]string{{"dotnet", "restore"}}, Label: "dotnet restore"})
	case name == "global.json":
		var g struct {
			SDK struct {
				Version string `json:"version"`
			} `json:"sdk"`
		}
		if json.Unmarshal([]byte(readFile(path)), &g) == nil && g.SDK.Version != "" {
			d.need("dotnet", rel)
			d.want("dotnet", g.SDK.Version, rel)
		}
	case name == "CMakeLists.txt":
		d.need("cmake", rel)
		d.need("cpp", rel)
	case name == "Makefile" || name == "makefile" || name == "GNUmakefile":
		d.need("make", rel)
		// Les recettes des Makefile ont besoin d'un sh : le terminal prend celui de Git.
		d.need("git", rel+" (sh des recettes)")
		if reMakeDocker.MatchString(readFile(path)) {
			d.need("docker", rel)
		}
	case name == "Dockerfile" || strings.HasSuffix(lower, ".dockerfile") ||
		name == "compose.yaml" || name == "compose.yml" || name == "docker-compose.yml" || name == "docker-compose.yaml":
		d.need("docker", rel)
	case strings.HasSuffix(lower, ".tf"):
		d.need("terraform", dirOf(rel)+"/*.tf")
	case name == "Chart.yaml":
		d.need("helm", rel)
	case strings.HasSuffix(lower, ".hurl"):
		d.need("hurl", dirOf(rel)+"/*.hurl")
	case name == "mix.exs":
		d.need("elixir", rel)
		d.need("erlang", rel)
	case name == "rebar.config":
		d.need("erlang", rel)
	case name == "pubspec.yaml":
		d.need("dart", rel)
	case name == "Package.swift":
		d.need("swift", rel)
	case name == "cpanfile" || name == "Makefile.PL":
		d.need("perl", rel)
	case name == "stack.yaml":
		d.need("haskell", rel)
	case name == "build.sbt":
		d.need("sbt", rel)
		d.need("java", rel)
	case name == ".tool-versions" || name == "mise.toml" || name == ".mise.toml":
		d.toolVersions(path, rel)
	}
}

// toolVersions lit .tool-versions (asdf) ou mise.toml : « nodejs 20.11.0 », node = "20".
func (d *detector) toolVersions(path, rel string) {
	names := map[string]string{
		"nodejs": "node", "node": "node", "python": "python", "golang": "go", "go": "go", "rust": "rust",
		"ruby": "ruby", "java": "java", "php": "php", "deno": "deno", "bun": "bun", "pnpm": "pnpm",
		"yarn": "yarn", "terraform": "terraform", "helm": "helm", "dotnet": "dotnet", "dotnet-core": "dotnet",
		"elixir": "elixir", "erlang": "erlang", "uv": "uv", "dart": "dart",
	}
	sc := bufio.NewScanner(strings.NewReader(readFile(path)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		var name, ver string
		if k, v, ok := strings.Cut(line, "="); ok { // mise.toml : node = "20"
			name, ver = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'[]`)
		} else if f := strings.Fields(line); len(f) >= 2 { // .tool-versions : nodejs 20.11.0
			name, ver = f[0], f[1]
		}
		if key, ok := names[strings.ToLower(name)]; ok {
			d.need(key, rel)
			d.want(key, ver, rel)
		}
	}
}

func (d *detector) hasStep(dir, label string) bool {
	for _, s := range d.p.Steps {
		if s.Dir == dir && s.Label == label {
			return true
		}
	}
	return false
}

func (d *detector) addStepOnce(s Step) {
	if !d.hasStep(s.Dir, s.Label) {
		d.p.Steps = append(d.p.Steps, s)
	}
}

func dirOf(rel string) string {
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if dir == "" {
		return "."
	}
	return dir
}

// cleanVersion extrait la version d'une contrainte : ">=18.17" → "18.17", "v20" → "20",
// "^3.10" → "3.10", "lts/*" → "" (pas de version précise), "temurin-21" → "21".
func cleanVersion(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "lts") || s == "latest" || s == "stable" || s == "*" {
		return ""
	}
	return reVersionToken.FindString(s)
}

// dropNested : « dotnet restore » à la racine d'une solution restaure aussi ses sous-projets.
func dropNested(steps []Step) []Step {
	var out []Step
	for _, s := range steps {
		nested := false
		for _, o := range steps {
			if o.Label == s.Label && s.Label == "dotnet restore" && o.Dir != s.Dir && isUnder(s.Dir, o.Dir) {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, s)
		}
	}
	return out
}

func isUnder(dir, parent string) bool {
	return parent == "." || strings.HasPrefix(dir+"/", parent+"/")
}
