package deps

// La CI et le devcontainer d'un projet décrivent, de la main de ses auteurs, les outils et les
// versions avec lesquels il se construit : actions/setup-node avec node-version-file: .nvmrc,
// setup-python, setup-java… apt deps les lit comme les autres fichiers du projet.

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// setupActions : action GitHub → outil, et les clés « with » qui donnent sa version.
var setupActions = map[string]struct {
	tool        string
	versionKeys []string
	fileKeys    []string // fichier qui contient la version (.nvmrc, go.mod…)
}{
	"actions/setup-node":                     {"node", []string{"node-version"}, []string{"node-version-file"}},
	"actions/setup-python":                   {"python", []string{"python-version"}, []string{"python-version-file"}},
	"actions/setup-go":                       {"go", []string{"go-version"}, []string{"go-version-file"}},
	"actions/setup-java":                     {"java", []string{"java-version"}, []string{"java-version-file"}},
	"actions/setup-dotnet":                   {"dotnet", []string{"dotnet-version"}, []string{"global-json-file"}},
	"ruby/setup-ruby":                        {"ruby", []string{"ruby-version"}, nil},
	"shivammathur/setup-php":                 {"php", []string{"php-version"}, nil},
	"pnpm/action-setup":                      {"pnpm", []string{"version"}, nil},
	"oven-sh/setup-bun":                      {"bun", []string{"bun-version"}, nil},
	"denoland/setup-deno":                    {"deno", nil, nil},
	"dtolnay/rust-toolchain":                 {"rust", []string{"toolchain"}, nil},
	"actions-rust-lang/setup-rust-toolchain": {"rust", []string{"toolchain"}, nil},
	"hashicorp/setup-terraform":              {"terraform", []string{"terraform_version"}, nil},
	"azure/setup-helm":                       {"helm", []string{"version"}, nil},
	"microsoft/setup-msbuild":                {"msvc", nil, nil},
	"ilammy/msvc-dev-cmd":                    {"msvc", nil, nil},
	"docker/setup-buildx-action":             {"docker", nil, nil},
	"docker/build-push-action":               {"docker", nil, nil},
	"gradle/actions/setup-gradle":            {"java", nil, nil},
	"astral-sh/setup-uv":                     {"uv", []string{"version"}, nil},
	"erlef/setup-beam":                       {"elixir", []string{"elixir-version"}, nil},
	"subosito/flutter-action":                {"dart", nil, nil},
	"lukka/get-cmake":                        {"cmake", []string{"cmakeVersion"}, nil},
}

var (
	reStepStart = regexp.MustCompile(`^(\s*)-\s`)
	reUses      = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*uses:[ \t]*['"]?([A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+?)@([^\s'"#]+)`)
	reYAMLKey   = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*([A-Za-z0-9_-]+):[ \t]*(.*)$`)
	reMatrixRef = regexp.MustCompile(`\$\{\{\s*matrix\.([A-Za-z0-9_-]+)\s*\}\}`)
	reExpr      = regexp.MustCompile(`\$\{\{`)
)

// readCI lit les workflows GitHub Actions du projet.
func (d *detector) readCI(root string) {
	files, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	more, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yaml"))
	for _, f := range append(files, more...) {
		rel := ".github/workflows/" + filepath.Base(f)
		d.workflow(root, rel, readFile(f))
	}
}

func (d *detector) workflow(root, rel, content string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	// Découpe en étapes : chaque « - » ouvre un bloc qui va jusqu'au « - » suivant de même retrait.
	for i := 0; i < len(lines); i++ {
		m := reStepStart.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent := len(m[1])
		j := i + 1
		for ; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				continue
			}
			ind := len(l) - len(strings.TrimLeft(l, " "))
			if ind < indent || (ind == indent && reStepStart.MatchString(l)) || (ind <= indent && !strings.HasPrefix(strings.TrimSpace(l), "-")) {
				break
			}
		}
		block := strings.Join(lines[i:j], "\n")
		if u := reUses.FindStringSubmatch(lines[i] + "\n" + block); u != nil {
			d.ciStep(root, rel, content, u[1], u[2], block)
		}
	}
}

func (d *detector) ciStep(root, rel, content, action, ref, block string) {
	action = strings.ToLower(action)
	spec, ok := setupActions[action]
	if !ok {
		return
	}
	d.need(spec.tool, rel)
	with := map[string]string{}
	for _, m := range reYAMLKey.FindAllStringSubmatch(block, -1) {
		with[m[1]] = cleanYAMLValue(m[2])
	}
	if spec.tool == "rust" && with["toolchain"] == "" {
		with["toolchain"] = ref // dtolnay/rust-toolchain@1.75
	}
	for _, k := range spec.versionKeys {
		v := with[k]
		if v == "" {
			continue
		}
		pinned := true
		if mm := reMatrixRef.FindStringSubmatch(v); mm != nil {
			v, pinned = matrixMax(content, mm[1]), false
		} else if reExpr.MatchString(v) {
			continue // expression qu'on ne sait pas évaluer
		}
		d.want(spec.tool, v, rel)
		// Une version exacte de Node dans la CI : le projet est construit avec cette majeure.
		if spec.tool == "node" && pinned && !strings.ContainsAny(v, "x*<>=^~|") {
			d.pin("node", v, rel)
		}
	}
	for _, k := range spec.fileKeys {
		f := with[k]
		if f == "" || reExpr.MatchString(f) {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(f))
		switch filepath.Base(f) {
		case "go.mod":
			if m := reGoDirective.FindStringSubmatch(readFile(path)); m != nil {
				d.want("go", m[1], rel)
			}
		case "package.json":
			// déjà lu avec le projet
		default:
			v := readFile(path)
			d.want(spec.tool, v, f)
			if spec.tool == "node" {
				d.pin("node", v, f)
			}
		}
	}
}

func cleanYAMLValue(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return strings.Trim(v, `'"`)
}

// matrixMax : la version la plus récente d'une liste de matrice (node: [18, 20, 22] → 22).
func matrixMax(content, key string) string {
	re := regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(key) + `:[ \t]*(.*)$`)
	m := re.FindStringSubmatchIndex(content)
	if m == nil {
		return ""
	}
	inline := content[m[4]:m[5]]
	var values []string
	if strings.HasPrefix(strings.TrimSpace(inline), "[") {
		for _, v := range strings.Split(strings.Trim(strings.TrimSpace(inline), "[]"), ",") {
			values = append(values, cleanYAMLValue(v))
		}
	} else if strings.TrimSpace(inline) == "" {
		indent := m[3] - m[2]
		for _, l := range strings.Split(content[m[1]:], "\n")[1:] {
			t := strings.TrimSpace(l)
			ind := len(l) - len(strings.TrimLeft(l, " "))
			if t == "" {
				continue
			}
			if !strings.HasPrefix(t, "- ") || ind <= indent {
				break
			}
			values = append(values, cleanYAMLValue(strings.TrimPrefix(t, "- ")))
		}
	} else {
		values = []string{cleanYAMLValue(inline)}
	}
	best := ""
	for _, v := range values {
		if c := cleanVersion(v); c != "" && (best == "" || compareVersions(c, best) > 0) {
			best = c
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Devcontainer

// devFeatures : features officielles des devcontainers → outil.
var devFeatures = map[string]string{
	"node": "node", "python": "python", "go": "go", "java": "java", "dotnet": "dotnet", "rust": "rust",
	"ruby": "ruby", "php": "php", "docker-in-docker": "docker", "docker-outside-of-docker": "docker",
	"terraform": "terraform", "kubectl-helm-minikube": "helm", "git": "git", "deno": "deno",
}

// devImages : images officielles mcr.microsoft.com/devcontainers/<nom> → outil.
var devImages = map[string]string{
	"typescript-node": "node", "javascript-node": "node", "python": "python", "go": "go", "rust": "rust",
	"java": "java", "dotnet": "dotnet", "php": "php", "ruby": "ruby", "cpp": "cpp",
}

var reImage = regexp.MustCompile(`mcr\.microsoft\.com/(?:vscode/)?devcontainers/([a-z-]+)(?::([A-Za-z0-9._-]+))?`)

// readDevcontainer lit .devcontainer/devcontainer.json (et .devcontainer.json, .devcontainer/*/devcontainer.json).
func (d *detector) readDevcontainer(root string) {
	paths := []string{filepath.Join(root, ".devcontainer", "devcontainer.json"), filepath.Join(root, ".devcontainer.json")}
	sub, _ := filepath.Glob(filepath.Join(root, ".devcontainer", "*", "devcontainer.json"))
	for _, p := range append(paths, sub...) {
		if !exists(p) {
			continue
		}
		rel, _ := filepath.Rel(root, p)
		d.devcontainer(filepath.ToSlash(rel), readFile(p), filepath.Dir(p))
	}
}

func (d *detector) devcontainer(rel, content, dir string) {
	var dc struct {
		Image    string                     `json:"image"`
		Features map[string]json.RawMessage `json:"features"`
		Build    struct {
			Dockerfile string `json:"dockerfile"`
		} `json:"build"`
	}
	if json.Unmarshal([]byte(stripJSONC(content)), &dc) != nil {
		return
	}
	image := dc.Image
	if image == "" && dc.Build.Dockerfile != "" {
		// FROM mcr.microsoft.com/devcontainers/typescript-node:22
		image = readFile(filepath.Join(dir, filepath.FromSlash(dc.Build.Dockerfile)))
	}
	if m := reImage.FindStringSubmatch(image); m != nil {
		if tool, ok := devImages[m[1]]; ok {
			d.need(tool, rel)
			d.want(tool, imageVersion(m[2]), rel)
		}
	}
	for id, raw := range dc.Features {
		// ghcr.io/devcontainers/features/node:1
		name := id[strings.LastIndex(id, "/")+1:]
		if i := strings.Index(name, ":"); i >= 0 {
			name = name[:i]
		}
		tool, ok := devFeatures[name]
		if !ok {
			continue
		}
		d.need(tool, rel)
		var opts map[string]any
		if json.Unmarshal(raw, &opts) == nil {
			if v, ok := opts["version"].(string); ok {
				d.want(tool, v, rel)
			}
		}
	}
}

// imageVersion : « 1-22-bookworm » → « 22 », « 3.12-bullseye » → « 3.12 », « 22 » → « 22 ».
func imageVersion(tag string) string {
	parts := strings.Split(tag, "-")
	if len(parts) >= 3 && parts[0] != "" && cleanVersion(parts[1]) != "" {
		return cleanVersion(parts[1])
	}
	if len(parts) > 0 {
		return cleanVersion(parts[0])
	}
	return ""
}

// stripJSONC retire les commentaires (// et /* */) et les virgules finales d'un JSON « avec commentaires ».
func stripJSONC(s string) string {
	var b strings.Builder
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			b.WriteByte(c)
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++
		default:
			b.WriteByte(c)
		}
	}
	return regexp.MustCompile(`,(\s*[}\]])`).ReplaceAllString(b.String(), "$1")
}
