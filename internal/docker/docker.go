// Package docker fournit un Docker léger pour Windows, sans Docker Desktop :
// une mini machine Alpine Linux dans WSL2 qui fait tourner Docker Engine,
// démarrée à la première commande docker et arrêtée automatiquement quand elle ne sert plus.
package docker

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Distro est le nom de la machine WSL dédiée à Docker.
const Distro = "orvaen-docker"

// dataDir : où est stocké le disque virtuel de la machine.
func dataDir() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA% sous Windows
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "OrvaenTerminal", "docker-vm")
}

// ---------------------------------------------------------------------------
// Appels à wsl.exe

func wslEnv() []string {
	// WSL_UTF8 : sortie de wsl.exe en UTF-8 au lieu d'UTF-16.
	// WSLENV : transmet le délai d'inactivité à la machine Linux.
	env := append(os.Environ(), "WSL_UTF8=1")
	wslenv := "ORVAEN_DOCKER_IDLE/u"
	if cur := os.Getenv("WSLENV"); cur != "" {
		wslenv = cur + ":" + wslenv
	}
	return append(env, "WSLENV="+wslenv)
}

func wsl(args ...string) int {
	cmd := exec.Command("wsl.exe", args...)
	cmd.Env = wslEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return exitCode(cmd.Run())
}

func wslQuiet(args ...string) int {
	cmd := exec.Command("wsl.exe", args...)
	cmd.Env = wslEnv()
	return exitCode(cmd.Run())
}

func wslOutput(args ...string) (string, error) {
	cmd := exec.Command("wsl.exe", args...)
	cmd.Env = wslEnv()
	out, err := cmd.Output()
	// Anciennes versions de WSL : sortie en UTF-16 malgré WSL_UTF8.
	return strings.ReplaceAll(string(out), "\x00", ""), err
}

func inDistro(args ...string) []string {
	return append([]string{"-d", Distro, "-u", "root", "--exec"}, args...)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}

// ---------------------------------------------------------------------------
// État

// Installed indique si la machine Docker légère existe.
var Installed = func() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := wslOutput("--list", "--quiet")
	if err != nil {
		return false
	}
	return hasDistro(out, Distro)
}

func hasDistro(list, name string) bool {
	for _, line := range strings.Split(list, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), name) {
			return true
		}
	}
	return false
}

func engineRunning() bool { return wslQuiet(inDistro("sh", "-c", runningCheck)...) == 0 }

// ensureRunning démarre le moteur s'il est arrêté et attend qu'il réponde.
func ensureRunning() int {
	if engineRunning() {
		return 0
	}
	return startEngine()
}

// startEngine démarre le moteur (supposé arrêté) et attend qu'il réponde.
// Trois lancements de wsl.exe en tout : préparation, moteur, attente.
func startEngine() int {
	ui.Info("Démarrage du moteur Docker…")
	// Préparation : BuildKit et wsl.conf. Le moteur est arrêté : aucun conteneur n'est interrompu.
	if wsl(inDistro("sh", "-c", prepareScript)...) == prepareRestart {
		wslQuiet("--terminate", Distro)
	}
	if err := startDetached("wsl.exe", inDistro("/usr/local/bin/orvaen-dockerd"), wslEnv()); err != nil {
		ui.Errorf("impossible de démarrer le moteur Docker : %v", err)
		return 1
	}
	if wslQuiet(inDistro("sh", "-c", waitReadyScript)...) == 0 {
		return 0
	}
	ui.Errorf("le moteur Docker ne répond pas. Consulte le journal : docker engine logs")
	return 1
}

// ---------------------------------------------------------------------------
// Commande docker

// Run exécute une commande docker dans la machine légère (ou « docker engine ... »).
func Run(args []string) int {
	if len(args) > 0 && args[0] == "engine" {
		return engine(args[1:])
	}
	if runtime.GOOS != "windows" {
		ui.Errorf("ce Docker léger n'est disponible que sous Windows.")
		return 1
	}
	if !installedFast() {
		ui.Errorf("Docker n'est pas installé. Lance : apt install docker")
		return 1
	}
	wargs := []string{"-d", Distro, "-u", "root"}
	if cwd, err := os.Getwd(); err == nil {
		wargs = append(wargs, "--cd", cwd)
		if hint := buildHint(args, cwd); hint != "" {
			ui.Warn(hint)
		}
	}
	wargs = append(append(wargs, "--exec", "sh", "-c", cliWrapper, "docker"), convertArgs(args)...)
	// Un seul lancement de wsl.exe quand le moteur tourne déjà : le script vérifie lui-même que
	// dockerd est prêt, et rend le code engineStopped sinon (rien n'a alors été lancé).
	code := wsl(wargs...)
	if code == engineStopped && !engineRunning() {
		if code := startEngine(); code != 0 {
			return code
		}
		code = wsl(wargs...)
	}
	return code
}

// engineStopped : code rendu par cliWrapper quand le moteur n'est pas démarré.
const engineStopped = 222

// installedFast : le disque de la machine existe (sans lancer wsl --list, qui prend du temps).
func installedFast() bool {
	if _, err := os.Stat(filepath.Join(dataDir(), "ext4.vhdx")); err == nil {
		return true
	}
	return Installed()
}

// buildHint : pour « docker build » lancé dans un dossier sans .dockerignore mais avec un gros dossier
// (node_modules, .git…), un conseil. Le contexte de build est lu depuis Windows à travers /mnt/c,
// ce qui est lent pour des milliers de petits fichiers.
func buildHint(args []string, dir string) string {
	isBuild := len(args) > 0 && args[0] == "build" ||
		len(args) > 1 && (args[0] == "buildx" || args[0] == "image") && args[1] == "build"
	if !isBuild {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, ".dockerignore")); err == nil {
		return ""
	}
	for _, heavy := range []string{"node_modules", ".git", "vendor", "target", ".venv", "dist"} {
		if info, err := os.Stat(filepath.Join(dir, heavy)); err == nil && info.IsDir() {
			return "pas de .dockerignore : " + heavy + " est envoyé au build à chaque fois, ce qui est lent depuis Windows. " +
				"Crée un fichier .dockerignore qui contient « " + heavy + " »."
		}
	}
	return ""
}

var drivePath = regexp.MustCompile(`(^|[=,])([A-Za-z]):[\\/]`)

// convertArgs traduit les chemins Windows (C:\projet) en chemins de la machine Linux (/mnt/c/projet),
// par exemple dans -v C:\projet:/app ou --mount source=C:\data,target=/data.
func convertArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if drivePath.MatchString(a) {
			a = drivePath.ReplaceAllStringFunc(a, func(m string) string {
				prefix := ""
				if m[0] == '=' || m[0] == ',' {
					prefix, m = m[:1], m[1:]
				}
				return prefix + "/mnt/" + strings.ToLower(m[:1]) + "/"
			})
			a = strings.ReplaceAll(a, `\`, "/")
		}
		out[i] = a
	}
	return out
}

func engine(args []string) int {
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	if sub != "" && sub != "help" && !Installed() {
		ui.Errorf("Docker n'est pas installé. Lance : apt install docker")
		return 1
	}
	switch sub {
	case "status":
		if engineRunning() {
			ui.Info("Moteur Docker : en marche (arrêt automatique après " + IdleText() + " d'inactivité).")
		} else {
			ui.Info("Moteur Docker : arrêté (il démarre tout seul à la prochaine commande docker).")
		}
		if size, err := diskSize(); err == nil {
			ui.Info("Espace disque utilisé : " + size)
		}
		return 0
	case "start":
		return ensureRunning()
	case "stop":
		code := wsl("--terminate", Distro)
		if code == 0 {
			ui.Info("Moteur Docker arrêté, la mémoire est libérée.")
		}
		return code
	case "logs":
		return wsl(inDistro("tail", "-n", "60", "/var/log/docker.log")...)
	default:
		fmt.Print(`Usage : docker engine <commande>

  status   État du moteur et espace disque utilisé
  start    Démarre le moteur (automatique à la première commande docker)
  stop     Arrête le moteur et libère la mémoire
  logs     Affiche le journal du moteur

Arrêt automatique après 10 min sans commande ni conteneur actif.
Pour changer le délai : variable d'environnement ORVAEN_DOCKER_IDLE en secondes (0 = jamais).
`)
		if sub == "" || sub == "help" {
			return 0
		}
		return 1
	}
}

func diskSize() (string, error) {
	var total int64
	err := filepath.Walk(dataDir(), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.1f Go", float64(total)/(1<<30)), nil
}

// ---------------------------------------------------------------------------
// Installation, mise à jour, désinstallation

// wslReady indique si WSL2 est installé et utilisable.
func wslReady() bool {
	if _, err := exec.LookPath("wsl.exe"); err != nil {
		return false
	}
	return wslQuiet("--status") == 0
}

// SetupWSL active WSL2 (droits admin nécessaires, redémarrage ensuite).
func SetupWSL() int {
	ui.Step("Activation de WSL2")
	code := wsl("--install", "--no-distribution")
	if code == 0 {
		ui.Warn("Redémarre ton PC, puis relance « apt install docker » pour terminer.")
	}
	return code
}

// Install crée la machine Docker légère. elevate relance « apt <args> » en administrateur.
func Install(elevate func(args ...string) int) int {
	if runtime.GOOS != "windows" {
		ui.Errorf("ce Docker léger n'est disponible que sous Windows.")
		return 1
	}
	if Installed() {
		ui.Info("Docker est déjà installé. Mise à jour : apt upgrade docker")
		return 0
	}

	ui.Step("Vérification de WSL2")
	if !wslReady() {
		if platform.IsElevated() {
			return SetupWSL()
		}
		ui.Info("WSL2 n'est pas encore activé : ouverture d'une fenêtre administrateur…")
		if code := elevate("wsl-setup", "--pause"); code != 0 {
			return code
		}
		return 0
	}

	tmp, err := os.MkdirTemp("", "orvaen-docker-")
	if err != nil {
		ui.Errorf("%v", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	ui.Step("Téléchargement d'Alpine Linux (quelques Mo)")
	tar, err := downloadAlpine(tmp)
	if err != nil {
		ui.Errorf("%v", err)
		return 1
	}

	ui.Step("Création de la machine Linux légère")
	dir := dataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		ui.Errorf("%v", err)
		return 1
	}
	if code := wsl("--import", Distro, dir, tar, "--version", "2"); code != 0 {
		ui.Errorf("la création de la machine WSL a échoué (code %d)", code)
		return code
	}

	ui.Step("Installation de Docker Engine (sans Docker Desktop)")
	if code := wsl(inDistro("sh", "-c", setupScript)...); code != 0 {
		ui.Errorf("l'installation de Docker Engine a échoué. Pour recommencer : apt remove docker -y && apt install docker")
		return code
	}

	// Redémarre la machine pour appliquer /etc/wsl.conf (montage de /mnt/c avec metadata).
	wslQuiet("--terminate", Distro)

	ui.Step("Premier démarrage du moteur")
	if code := ensureRunning(); code != 0 {
		return code
	}
	wsl(inDistro("docker", "--version")...)
	ui.Info("Docker est prêt ! Essaie :  docker run --rm hello-world")
	return 0
}

// Upgrade met à jour Docker Engine et Alpine dans la machine.
func Upgrade() int {
	if !Installed() {
		ui.Errorf("Docker n'est pas installé. Lance : apt install docker")
		return 1
	}
	ui.Step("Mise à jour de Docker Engine")
	code := wsl(inDistro("sh", "-c", "apk add --no-cache -q docker-cli-buildx && apk upgrade --no-cache")...)
	if code == 0 && engineRunning() {
		ui.Info("Pour utiliser la nouvelle version tout de suite : docker engine stop")
	}
	return code
}

// Uninstall supprime la machine et tout son contenu, après confirmation (sauf si yes).
func Uninstall(yes bool) int {
	if !Installed() {
		ui.Info("Docker léger n'est pas installé.")
		return 0
	}
	if !yes {
		fmt.Print("Supprimer la machine Docker avec TOUS ses conteneurs, images et volumes ? [o/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "o", "oui", "y", "yes":
		default:
			ui.Info("Annulé.")
			return 1
		}
	}
	ui.Step("Suppression de la machine Docker")
	if code := wsl("--unregister", Distro); code != 0 {
		return code
	}
	_ = os.RemoveAll(dataDir())
	ui.Info("Docker léger est désinstallé.")
	return 0
}

// IdleText : le délai d'arrêt automatique du moteur (ORVAEN_DOCKER_IDLE, 600 s par défaut), lisible.
func IdleText() string {
	n, err := strconv.Atoi(os.Getenv("ORVAEN_DOCKER_IDLE"))
	switch {
	case err != nil || n < 0:
		return "10 min"
	case n == 0:
		return "jamais"
	case n%3600 == 0:
		return fmt.Sprintf("%d h", n/3600)
	case n >= 60:
		return fmt.Sprintf("%d min", n/60)
	}
	return fmt.Sprintf("%d s", n)
}

// Subcommands : sous-commandes docker proposées par la complétion (Tab).
var Subcommands = []string{
	"run", "ps", "images", "pull", "push", "build", "compose", "exec", "logs", "stop", "start",
	"restart", "rm", "rmi", "inspect", "network", "volume", "system", "login", "engine",
}
