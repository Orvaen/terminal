package deps

// Modules Node natifs : des dépendances écrites en C/C++ (node-pty, sqlite3, bcrypt…), compilées
// par node-gyp pendant « npm install » quand aucun binaire précompilé n'existe pour la version de
// Node. Sous Windows, node-gyp exige Python et le compilateur MSVC : sans eux, l'installation
// échoue au bout de plusieurs minutes avec une erreur peu claire. apt deps les réclame avant.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// nativeHelpers : dépendances qui trahissent un module natif.
var nativeHelpers = []string{"nan", "node-addon-api", "node-gyp", "bindings", "prebuild-install", "node-pre-gyp", "@mapbox/node-pre-gyp"}

// scriptOnly : paquets connus qui ont un script d'installation sans rien compiler
// (téléchargement d'un binaire, message, hooks Git…).
var scriptOnly = map[string]bool{
	"husky": true, "es5-ext": true, "core-js": true, "core-js-pure": true, "esbuild": true,
	"electron": true, "puppeteer": true, "playwright": true, "playwright-core": true, "protobufjs": true,
	"nodemon": true, "cypress": true, "ngrok": true, "koffi": true, "msw": true, "spawn-sync": true,
	"@sentry/cli": true, "prisma": true, "@prisma/client": true, "@prisma/engines": true, "sharp": true,
	"@biomejs/biome": true, "lefthook": true, "simple-git-hooks": true, "yarn": true, "pnpm": true,
	"@swc/core": true, "turbo": true, "nx": true, "vue-demi": true, "styled-components": true,
}

// lockPackage : une entrée « packages » d'un package-lock.json (v2/v3).
type lockPackage struct {
	HasInstallScript     bool              `json:"hasInstallScript"`
	OS                   []string          `json:"os"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

// nativeModules renvoie les modules natifs d'un dossier Node (package.json) : d'abord ceux de
// node_modules (binding.gyp), sinon ceux que le package-lock.json annonce.
func nativeModules(dir string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	// Le projet lui-même est un module natif.
	if exists(filepath.Join(dir, "binding.gyp")) {
		add("(ce projet : binding.gyp)")
	}

	if nm := filepath.Join(dir, "node_modules"); exists(nm) {
		entries, _ := os.ReadDir(nm)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if strings.HasPrefix(e.Name(), "@") {
				scoped, _ := os.ReadDir(filepath.Join(nm, e.Name()))
				for _, s := range scoped {
					if exists(filepath.Join(nm, e.Name(), s.Name(), "binding.gyp")) {
						add(e.Name() + "/" + s.Name())
					}
				}
				continue
			}
			if exists(filepath.Join(nm, e.Name(), "binding.gyp")) {
				add(e.Name())
			}
		}
		sort.Strings(out)
		return out
	}

	var lock struct {
		Packages map[string]lockPackage `json:"packages"`
	}
	if json.Unmarshal([]byte(readFile(filepath.Join(dir, "package-lock.json"))), &lock) != nil {
		return out
	}
	for key, p := range lock.Packages {
		if !p.HasInstallScript || key == "" || !allowsWindows(p.OS) {
			continue
		}
		name := key[strings.LastIndex(key, "node_modules/")+len("node_modules/"):]
		if scriptOnly[name] || strings.HasPrefix(name, "@esbuild/") || strings.HasPrefix(name, "@playwright/") {
			continue
		}
		helpers := 0
		onlyGypBuild := false
		for dep := range mergeDeps(p.Dependencies, p.OptionalDependencies) {
			for _, h := range nativeHelpers {
				if dep == h {
					helpers++
				}
			}
			if dep == "node-gyp-build" {
				onlyGypBuild = true
			}
		}
		// node-gyp-build seul : le paquet livre ses binaires précompilés (prebuildify).
		if onlyGypBuild && helpers == 0 {
			continue
		}
		add(name)
	}
	sort.Strings(out)
	return out
}

func mergeDeps(a, b map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}

// allowsWindows : le champ « os » d'un paquet n'exclut pas Windows (fsevents est réservé à macOS).
func allowsWindows(osList []string) bool {
	if len(osList) == 0 {
		return true
	}
	allowed := false
	for _, o := range osList {
		switch {
		case o == "win32":
			allowed = true
		case o == "!win32":
			return false
		case !strings.HasPrefix(o, "!"):
			// liste positive sans win32 : exclu, sauf si win32 apparaît plus loin
		default:
			allowed = true // seulement des exclusions, dont aucune de Windows
		}
	}
	return allowed
}

// summarize : « node-pty, @vscode/sqlite3, kerberos et 12 autres ».
func summarize(names []string, max int) string {
	if len(names) <= max+1 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + " et " + strconv.Itoa(len(names)-max) + " autres"
}
