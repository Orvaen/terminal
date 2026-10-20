package shell

// Coreutils pour Windows (Microsoft.Coreutils, « apt install coreutils ») : les commandes Unix
// d'uutils (ls, grep, head, sort, find, xargs…), natives pour Windows, dans un seul binaire.
//
// Quand il est installé, le terminal :
//   - met son dossier en tête du PATH : find, sort, date, hostname… sont ceux de Linux, pas ceux
//     de System32 (comme dans Git Bash) ;
//   - garde ses commandes intégrées (ls, rm, cp…) pour les usages simples, et passe la main à
//     Coreutils pour les options qu'elles ne gèrent pas (ls --color -la, cat qui lit un tube…).

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Orvaen/terminal/internal/platform"
)

// coreutilsCommands : les commandes livrées par Coreutils pour Windows.
var coreutilsCommands = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`arch b2sum base32 base64 basename basenc cat cksum comm cp csplit cut
		date df dirname du echo env expr factor false find fmt fold grep head hostname join la link ln ls
		md5sum mkdir mktemp mv nl nproc numfmt od pathchk pr printenv printf ptx pwd readlink realpath rm
		rmdir seq sha1sum sha224sum sha256sum sha384sum sha512sum shuf sleep sort split stat sum tac tail
		tee test touch tr true truncate tsort unexpand uniq unlink uptime wc xargs yes`) {
		coreutilsCommands[n] = true
	}
}

var core struct {
	sync.Mutex
	dir      string // dossier de Coreutils, "" s'il n'est pas installé
	detected bool
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// isCoreutilsDir : un dossier qui contient le gestionnaire ou le binaire multi-appel de Coreutils.
func isCoreutilsDir(dir string) bool {
	for _, n := range []string{"coreutils-manager", "coreutils"} {
		if info, err := os.Stat(filepath.Join(dir, exeName(n))); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// findCoreutilsDir cherche Coreutils : dans le PATH, dans le PATH enregistré (installé depuis
// l'ouverture du terminal), puis aux emplacements d'installation habituels.
func findCoreutilsDir(look func(string) (string, error), registryDirs, defaults []string) string {
	var candidates []string
	for _, n := range []string{"coreutils-manager", "coreutils"} {
		if p, err := look(n); err == nil {
			candidates = append(candidates, filepath.Dir(p))
		}
	}
	candidates = append(candidates, registryDirs...)
	candidates = append(candidates, defaults...)
	for _, d := range candidates {
		if d != "" && isCoreutilsDir(d) {
			return d
		}
	}
	return ""
}

func defaultCoreutilsDirs() []string {
	var dirs []string
	for _, base := range []string{os.Getenv("LOCALAPPDATA") + `\Programs`, os.Getenv("ProgramFiles")} {
		if base == "" || base == `\Programs` {
			continue
		}
		for _, name := range []string{"Coreutils", "Microsoft Coreutils", `Microsoft\Coreutils`} {
			dirs = append(dirs, filepath.Join(base, name))
		}
	}
	return dirs
}

// setupCoreutils détecte Coreutils et le met en tête du PATH du terminal.
// Appelée au démarrage et après chaque commande apt (il vient peut-être d'être installé).
func setupCoreutils() {
	if runtime.GOOS != "windows" || os.Getenv("ORVAEN_NO_COREUTILS") != "" {
		return
	}
	dir := findCoreutilsDir(exec.LookPath, platform.RegistryPathDirs(), defaultCoreutilsDirs())
	core.Lock()
	changed := dir != core.dir
	core.dir, core.detected = dir, true
	core.Unlock()
	if dir != "" {
		os.Setenv("PATH", withFirst(os.Getenv("PATH"), dir))
	}
	if changed {
		resetCommandNames()
	}
}

// coreutilsDir renvoie le dossier de Coreutils, ou "" s'il n'est pas installé.
func coreutilsDir() string {
	core.Lock()
	defer core.Unlock()
	return core.dir
}

// coreutilsTool renvoie de quoi lancer la commande Coreutils name : son .exe, ou le binaire
// multi-appel suivi du nom de la commande.
func coreutilsTool(name string) (path string, pre []string, ok bool) {
	dir := coreutilsDir()
	if dir == "" || !coreutilsCommands[name] {
		return "", nil, false
	}
	if p := filepath.Join(dir, exeName(name)); fileExists(p) {
		return p, nil, true
	}
	if p := filepath.Join(dir, exeName("coreutils")); fileExists(p) {
		return p, []string{name}, true
	}
	return "", nil, false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// withFirst place dir en tête du PATH (en retirant un éventuel doublon).
func withFirst(path, dir string) string {
	parts := []string{dir}
	for _, p := range filepath.SplitList(path) {
		if p != "" && !strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

// builtinFlags : options gérées par les commandes intégrées qui existent aussi dans Coreutils.
var builtinFlags = map[string]string{
	"ls": "alh1", "cat": "", "touch": "", "mkdir": "p", "rm": "rRfv",
	"rmdir": "", "cp": "rRv", "mv": "v", "echo": "n",
}

// needsCoreutils indique si une commande intégrée doit céder la place à Coreutils :
// options qu'elle ne connaît pas (--color, -t, cat -n…).
func needsCoreutils(name string, args []string) bool {
	allowed, ok := builtinFlags[name]
	if name == "env" {
		return len(args) > 0 // env VAR=x commande
	}
	if !ok {
		return false
	}
	for _, a := range args {
		if a == "--" {
			return false
		}
		if strings.HasPrefix(a, "--") {
			return true
		}
		if name == "echo" {
			if a == "-e" || a == "-E" || a == "-ne" || a == "-en" {
				return true
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for _, r := range a[1:] {
				if !strings.ContainsRune(allowed, r) {
					return true
				}
			}
		}
	}
	return false
}
