package apt

// apt java : plusieurs JDK côte à côte, sans droits administrateur, comme SDKMAN sous Linux.
//
// Les JDK viennent d'Eclipse Adoptium (Temurin) en zip, vérifiés par leur somme SHA-256, et sont
// rangés dans %LOCALAPPDATA%\OrvaenTerminal\outils\jdk\<version> (jdk\21, jdk\25…). « apt java use »
// choisit le JDK actif : JAVA_HOME et le PATH de l'utilisateur pointent dessus. Un dossier par
// version majeure : une mise à jour (21.0.12 → 21.0.13) garde le même JAVA_HOME.
//
// Un Java installé pour tout l'ordinateur (Oracle, par exemple) reste en place : son dossier est dans
// le PATH système, qui passe avant celui de l'utilisateur dans PowerShell et cmd. Dans le terminal,
// le JDK actif passe en tête (ActiveJDKBin) ; Gradle et Maven, eux, suivent JAVA_HOME partout.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Points d'injection, remplacés dans les tests.
var (
	adoptiumAPI   = "https://api.adoptium.net/v3"
	readUserPath  = func() []platform.PathEntry { return platform.RegistryPath("user") }
	writeUserPath = func(raw []string) error { return platform.WritePath("user", raw) }
	systemJavaDir = func() string {
		for _, e := range platform.RegistryPath("system") {
			if fileExists(filepath.Join(e.Dir, "java.exe")) {
				return e.Dir
			}
		}
		return ""
	}
)

func jdkRoot() string          { return filepath.Join(toolsRoot(), "jdk") }
func jdkDir(major int) string  { return filepath.Join(jdkRoot(), strconv.Itoa(major)) }
func jdkBin(major int) string  { return filepath.Join(jdkDir(major), "bin") }
func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
func jdkJava(major int) string { return filepath.Join(jdkBin(major), javaExe()) }
func javaExe() string {
	if runtime.GOOS == "windows" {
		return "java.exe"
	}
	return "java"
}
func jdkLabel(major int) string { return "JDK " + strconv.Itoa(major) }
func javaUsage() string {
	return "apt java [list|installed|install|use|remove|upgrade|current] [version]"
}
func jdkVersion(major int) (string, bool) {
	b, err := os.ReadFile(filepath.Join(jdkDir(major), ".version"))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// installedJDKs : les versions installées par apt, de la plus récente à la plus ancienne.
func installedJDKs() []int {
	entries, _ := os.ReadDir(jdkRoot())
	var out []int
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err == nil && e.IsDir() {
			if _, ok := jdkVersion(n); ok {
				out = append(out, n)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// activeJDK : la version gérée par apt vers laquelle pointe JAVA_HOME, ou 0.
func activeJDK() int {
	home := os.Getenv("JAVA_HOME")
	if home == "" {
		return 0
	}
	for _, n := range installedJDKs() {
		if platform.SamePath(home, jdkDir(n)) {
			return n
		}
	}
	return 0
}

// ActiveJDKBin : le dossier bin du JDK actif géré par apt, à placer en tête du PATH au démarrage du
// terminal (avant le Java installé pour tout l'ordinateur). "" s'il n'y en a pas.
func ActiveJDKBin() string {
	if n := activeJDK(); n != 0 && fileExists(jdkJava(n)) {
		return jdkBin(n)
	}
	return ""
}

// ---------------------------------------------------------------------------
// Noms de paquets : java, jdk21, openjdk-21-jdk, temurin-17…

var (
	javaPlain   = map[string]bool{"java": true, "jdk": true, "openjdk": true, "temurin": true, "default-jdk": true, "default-jre": true, "jre": true}
	javaNumbers = regexp.MustCompile(`^(?:java|jdk|jre|openjdk|temurin)-?(\d+)(?:-jdk|-jre)?$`)
)

// javaPackage : le nom désigne un JDK. major vaut 0 pour « la version par défaut ».
func javaPackage(name string) (major int, ok bool) {
	n := strings.ToLower(name)
	if javaPlain[n] {
		return 0, true
	}
	if m := javaNumbers.FindStringSubmatch(n); m != nil {
		v, _ := strconv.Atoi(m[1])
		return v, true
	}
	return 0, false
}

// useJavaPortable : « apt install java » sans droits administrateur passe par apt java (le JDK de
// winget s'installe pour tout l'ordinateur et demanderait l'élévation).
func useJavaPortable(name string) (int, bool) {
	if !portableEnabled || isElevated() {
		return 0, false
	}
	return javaPackage(name)
}

// ---------------------------------------------------------------------------
// Eclipse Adoptium

type adoptiumReleases struct {
	LTS        []int `json:"available_lts_releases"`
	All        []int `json:"available_releases"`
	RecentLTS  int   `json:"most_recent_lts"`
	RecentRels int   `json:"most_recent_feature_release"`
}

func javaReleases() (adoptiumReleases, error) {
	var r adoptiumReleases
	if err := getJSON(adoptiumAPI+"/info/available_releases", &r); err != nil {
		return r, err
	}
	if r.RecentLTS == 0 || len(r.All) == 0 {
		return r, errors.New("réponse d'api.adoptium.net incomplète")
	}
	return r, nil
}

type jdkAsset struct {
	Release string
	Link    string
	Sum     string
}

func adoptiumArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "386":
		return "x32"
	}
	return "x64"
}

func latestJDK(major int) (jdkAsset, error) {
	var assets []struct {
		Binary struct {
			Package struct {
				Link     string `json:"link"`
				Checksum string `json:"checksum"`
			} `json:"package"`
		} `json:"binary"`
		Release string `json:"release_name"`
	}
	url := fmt.Sprintf("%s/assets/latest/%d/hotspot?architecture=%s&image_type=jdk&os=windows&vendor=eclipse", adoptiumAPI, major, adoptiumArch())
	if err := getJSON(url, &assets); err != nil {
		return jdkAsset{}, err
	}
	for _, a := range assets {
		p := a.Binary.Package
		if strings.HasSuffix(strings.ToLower(p.Link), ".zip") && p.Checksum != "" && a.Release != "" {
			return jdkAsset{Release: a.Release, Link: p.Link, Sum: p.Checksum}, nil
		}
	}
	return jdkAsset{}, fmt.Errorf("pas de JDK %d pour Windows %s chez Eclipse Adoptium", major, adoptiumArch())
}

// ---------------------------------------------------------------------------
// Commandes

func cmdJava(args []string) int {
	sub, rest := "list", args
	if len(args) > 0 {
		sub, rest = strings.ToLower(args[0]), args[1:]
	}
	switch sub {
	case "list", "ls-remote", "available":
		return javaList()
	case "installed", "ls":
		return javaInstalled()
	case "install", "add":
		return javaInstallCmd(rest)
	case "use", "default", "switch":
		return javaUse(rest)
	case "remove", "rm", "uninstall", "purge":
		return javaRemoveCmd(rest)
	case "upgrade", "update":
		return javaUpgrade(rest)
	case "current", "which", "active":
		return javaCurrent()
	case "help", "-h", "--help":
		printJavaHelp()
		return 0
	}
	ui.Errorf("action inconnue : apt java %s", sub)
	printJavaHelp()
	return 1
}

func printJavaHelp() {
	fmt.Fprintf(stdout, `Usage : %s

  list                 Versions disponibles (LTS et la plus récente), et celles installées
  installed            Versions installées par apt (* : l'active)
  install [version]    Installe un JDK (par défaut : la dernière LTS) ; le premier devient l'actif
  use <version>        Choisit le JDK actif : JAVA_HOME et PATH de ton compte, sans droits admin
  remove <version>     Désinstalle un JDK
  upgrade [version]    Met à jour les JDK installés (même version majeure : 21.0.12 → 21.0.13)
  current              Le JDK actif

Versions : 21, jdk21, openjdk-21, temurin-21, lts, latest. Source : Eclipse Adoptium (Temurin).
`, javaUsage())
}

// parseJavaVersion : « 21 », « jdk-21 », « lts », « latest », ou "" (dernière LTS).
func parseJavaVersion(arg string) (int, error) {
	a := strings.ToLower(strings.TrimSpace(arg))
	if a == "" || a == "lts" || javaPlain[a] {
		r, err := javaReleases()
		return r.RecentLTS, err
	}
	if a == "latest" || a == "dernière" || a == "derniere" {
		r, err := javaReleases()
		return r.RecentRels, err
	}
	if v, err := strconv.Atoi(strings.SplitN(a, ".", 2)[0]); err == nil && v > 0 {
		return v, nil
	}
	if v, ok := javaPackage(a); ok && v > 0 {
		return v, nil
	}
	return 0, fmt.Errorf("version inconnue : %s (exemples : 21, 25, lts)", arg)
}

// installedArg : la version installée désignée par arg ; sans argument, la seule installée.
func installedArg(args []string, verb string) (int, bool) {
	inst := installedJDKs()
	if len(args) == 0 {
		if len(inst) == 1 {
			return inst[0], true
		}
		if len(inst) == 0 {
			ui.Errorf("aucun JDK installé par apt : apt java install 21")
		} else {
			ui.Errorf("précise la version : apt java %s %d (installées : %s)", verb, inst[0], joinInts(inst))
		}
		return 0, false
	}
	n, err := parseJavaVersion(args[0])
	if err != nil {
		ui.Errorf("%v", err)
		return 0, false
	}
	if _, ok := jdkVersion(n); !ok {
		ui.Errorf("le JDK %d n'est pas installé : apt java install %d", n, n)
		return 0, false
	}
	return n, true
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ", ")
}

func javaList() int {
	inst := installedJDKs()
	active := activeJDK()
	r, err := javaReleases()
	if err != nil {
		ui.Warn("api.adoptium.net injoignable (" + err.Error() + ") : seulement les versions installées.")
		return javaInstalled()
	}
	lts := map[int]bool{}
	show := map[int]bool{r.RecentRels: true}
	for _, v := range r.LTS {
		lts[v], show[v] = true, true
	}
	for _, v := range inst {
		show[v] = true
	}
	var versions []int
	for v := range show {
		versions = append(versions, v)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))

	fmt.Fprintln(stdout, "Versions de Java (Temurin, Eclipse Adoptium) :")
	for _, v := range versions {
		tag := "   "
		if lts[v] {
			tag = "LTS"
		}
		state := ""
		if rel, ok := jdkVersion(v); ok {
			state = "installé (" + rel + ")"
			if v == active {
				state += ", actif"
			}
			state = ui.Color("32", state)
		} else if v == r.RecentLTS {
			state = "← conseillé"
		}
		mark := " "
		if v == active {
			mark = "*"
		}
		fmt.Fprintf(stdout, " %s %3d  %s  %s\n", mark, v, tag, state)
	}
	fmt.Fprintln(stdout, "\nInstaller : apt java install <version>    Choisir l'actif : apt java use <version>")
	return 0
}

func javaInstalled() int {
	inst := installedJDKs()
	if len(inst) == 0 {
		ui.Info("Aucun JDK installé par apt. Pour en installer un : apt java install (dernière LTS) ou apt java install 21")
		return 0
	}
	active := activeJDK()
	for _, n := range inst {
		rel, _ := jdkVersion(n)
		mark := " "
		if n == active {
			mark = "*"
		}
		fmt.Fprintf(stdout, " %s %3d  %-18s %s\n", mark, n, rel, jdkDir(n))
	}
	if active == 0 {
		fmt.Fprintln(stdout, "\nAucun n'est actif : apt java use <version>")
	}
	return 0
}

func javaCurrent() int {
	if n := activeJDK(); n != 0 {
		rel, _ := jdkVersion(n)
		fmt.Fprintf(stdout, "JDK %d (%s)\n  JAVA_HOME = %s\n", n, rel, jdkDir(n))
		return 0
	}
	if home := os.Getenv("JAVA_HOME"); home != "" {
		fmt.Fprintf(stdout, "JAVA_HOME = %s (pas installé par apt)\n", home)
		return 0
	}
	if p, err := toolLookPath("java"); err == nil {
		fmt.Fprintf(stdout, "Pas de JAVA_HOME. « java » est %s (pas installé par apt).\n", p)
		return 0
	}
	ui.Info("Aucun Java. Pour en installer un : apt java install")
	return 1
}

func javaInstallCmd(args []string) int {
	if len(args) == 0 {
		args = []string{""}
	}
	code := 0
	for _, a := range args {
		n, err := parseJavaVersion(a)
		if err != nil {
			ui.Errorf("%v", err)
			code = 1
			continue
		}
		if c := installJDK(n, false); c != 0 && code == 0 {
			code = c
		}
	}
	return code
}

// installJDK installe ou met à jour le JDK major. onlyIfInstalled : pour apt upgrade.
// Le premier JDK installé devient l'actif.
func installJDK(major int, onlyIfInstalled bool) int {
	have, installed := jdkVersion(major)
	if onlyIfInstalled && !installed {
		ui.Info(jdkLabel(major) + " n'a pas été installé par apt : rien à mettre à jour.")
		return 0
	}
	asset, err := latestJDK(major)
	if err != nil {
		ui.Errorf("%s : %v", jdkLabel(major), err)
		return 1
	}
	if installed && have == asset.Release {
		ui.Info(fmt.Sprintf("%s (%s) est déjà à jour.", jdkLabel(major), have))
		return ensureActive(major)
	}
	ui.Info(fmt.Sprintf("%s : %s, depuis Eclipse Adoptium (zip, somme SHA-256 vérifiée)", jdkLabel(major), asset.Release))

	// Installation dans un dossier voisin, puis échange : une installation ratée ne casse pas l'ancienne.
	dir := jdkDir(major)
	tmp, old := dir+".nouveau", dir+".ancien"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		ui.Errorf("%v", err)
		return 1
	}
	if err := downloadUnzip(asset.Link, tmp, asset.Sum); err != nil {
		os.RemoveAll(tmp)
		ui.Errorf("%s : %v", jdkLabel(major), err)
		return 1
	}
	if !fileExists(filepath.Join(tmp, "bin", javaExe())) {
		os.RemoveAll(tmp)
		ui.Errorf("%s : bin\\%s absent de l'archive", jdkLabel(major), javaExe())
		return 1
	}
	if err := os.WriteFile(filepath.Join(tmp, ".version"), []byte(asset.Release+"\n"), 0o644); err != nil {
		os.RemoveAll(tmp)
		ui.Errorf("%v", err)
		return 1
	}
	os.RemoveAll(old)
	if fileExists(dir) {
		if err := os.Rename(dir, old); err != nil {
			os.RemoveAll(tmp)
			ui.Errorf("%s est en cours d'utilisation (Gradle, un IDE ?) : ferme-le et recommence (%v)", jdkLabel(major), err)
			return 1
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		ui.Errorf("%v", err)
		return 1
	}
	os.RemoveAll(old)
	if installed {
		ui.Info(fmt.Sprintf("%s mis à jour : %s → %s.", jdkLabel(major), have, asset.Release))
	} else {
		ui.Info(fmt.Sprintf("%s est installé dans %s.", jdkLabel(major), dir))
	}
	return ensureActive(major)
}

// ensureActive : sans JDK actif géré par apt, major le devient. Sinon, rien ne change.
func ensureActive(major int) int {
	if a := activeJDK(); a != 0 {
		if a != major {
			ui.Info(fmt.Sprintf("Le JDK actif reste le %d. Pour passer au %d : apt java use %d", a, major, major))
		}
		return 0
	}
	return useJDK(major)
}

func javaUse(args []string) int {
	n, ok := installedArg(args, "use")
	if !ok {
		return 1
	}
	return useJDK(n)
}

// useJDK : JAVA_HOME et PATH de l'utilisateur (registre, pour les prochaines fenêtres) et du terminal.
func useJDK(major int) int {
	home, bin := jdkDir(major), jdkBin(major)
	if err := setUserEnv("JAVA_HOME", home, false); err != nil {
		ui.Errorf("impossible d'enregistrer JAVA_HOME : %v", err)
		return 1
	}
	// Le bin du JDK actif passe en tête du PATH utilisateur, les autres JDK d'apt en sortent.
	raw := []string{bin}
	for _, e := range readUserPath() {
		if !isJDKBin(e.Dir) {
			raw = append(raw, e.Raw)
		}
	}
	if err := writeUserPath(raw); err != nil {
		ui.Warn("impossible de modifier le PATH : " + err.Error())
	}
	os.Setenv("JAVA_HOME", home)
	setProcessJDK(bin)

	rel, _ := jdkVersion(major)
	ui.Info(fmt.Sprintf("JDK actif : %d (%s). JAVA_HOME = %s", major, rel, home))
	if d := systemJavaDir(); d != "" && !platform.SamePath(d, bin) {
		ui.Info("Un autre Java est installé pour tout l'ordinateur (" + d + ") : dans PowerShell et cmd, « java »\n" +
			"   reste celui-là. Dans le terminal, c'est le JDK " + strconv.Itoa(major) + ", et Gradle et Maven suivent JAVA_HOME partout.")
	}
	return 0
}

// isJDKBin : dir est le dossier bin d'un JDK installé par apt.
func isJDKBin(dir string) bool {
	rel, err := filepath.Rel(jdkRoot(), dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 2 || !strings.EqualFold(parts[1], "bin") {
		return false
	}
	_, err = strconv.Atoi(parts[0])
	return err == nil
}

// setProcessJDK : le PATH du terminal en cours, sans les autres JDK d'apt, avec bin en tête ("" : retirer).
func setProcessJDK(bin string) {
	var kept []string
	if bin != "" {
		kept = append(kept, bin)
	}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d != "" && !isJDKBin(d) && !platform.SamePath(d, bin) {
			kept = append(kept, d)
		}
	}
	os.Setenv("PATH", strings.Join(kept, string(os.PathListSeparator)))
}

func javaRemoveCmd(args []string) int {
	n, ok := installedArg(args, "remove")
	if !ok {
		return 1
	}
	return removeJDK(n)
}

func removeJDK(major int) int {
	wasActive := activeJDK() == major
	if err := os.RemoveAll(jdkDir(major)); err != nil {
		ui.Errorf("%s est en cours d'utilisation (Gradle, un IDE ?) : ferme-le et recommence (%v)", jdkLabel(major), err)
		return 1
	}
	if wasActive {
		_ = setUserEnv("JAVA_HOME", "", true)
		var raw []string
		for _, e := range readUserPath() {
			if !platform.SamePath(e.Dir, jdkBin(major)) {
				raw = append(raw, e.Raw)
			}
		}
		_ = writeUserPath(raw)
		os.Unsetenv("JAVA_HOME")
		setProcessJDK("")
	}
	ui.Info(jdkLabel(major) + " est désinstallé.")
	if rest := installedJDKs(); wasActive && len(rest) > 0 {
		ui.Info(fmt.Sprintf("Plus aucun JDK actif. Pour en choisir un : apt java use %d", rest[0]))
	}
	return 0
}

func javaUpgrade(args []string) int {
	targets := installedJDKs()
	if len(args) > 0 {
		n, ok := installedArg(args, "upgrade")
		if !ok {
			return 1
		}
		targets = []int{n}
	}
	if len(targets) == 0 {
		ui.Info("Aucun JDK installé par apt : rien à mettre à jour.")
		return 0
	}
	code := 0
	for _, n := range targets {
		if c := installJDK(n, true); c != 0 && code == 0 {
			code = c
		}
	}
	return code
}

// upgradeJDKs : pour « apt upgrade », met à jour les JDK qu'apt a installés.
func upgradeJDKs() int {
	code := 0
	for _, n := range installedJDKs() {
		ui.Step("Mise à jour du " + jdkLabel(n))
		if c := installJDK(n, true); c != 0 && code == 0 {
			code = c
		}
	}
	return code
}

// upgradeJava : « apt upgrade java » (ou jdk21…) quand des JDK ont été installés par apt java.
func upgradeJava(name string) (code int, handled bool) {
	v, ok := javaPackage(name)
	if !ok || len(installedJDKs()) == 0 {
		return 0, false
	}
	if v == 0 {
		return javaUpgrade(nil), true
	}
	if _, inst := jdkVersion(v); inst {
		return installJDK(v, true), true
	}
	return 0, false
}

// versionArg : 0 (version par défaut) → "", sinon "21".
func versionArg(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func versionArgs(v int) []string {
	if v == 0 {
		return nil
	}
	return []string{strconv.Itoa(v)}
}
