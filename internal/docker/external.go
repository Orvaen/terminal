package docker

// Moteur choisi par l'utilisateur.
//
// Par défaut, docker parle au moteur de la machine Orvaen. Mais si l'utilisateur a désigné un autre moteur
// avec DOCKER_HOST ou DOCKER_CONTEXT (Docker Desktop, un serveur distant, un autre WSL…), on le respecte,
// comme le client docker officiel : la commande part telle quelle vers ce moteur par le client docker
// Windows, sans démarrer ni toucher la machine Orvaen.

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Orvaen/terminal/internal/ui"
)

// externalEngine rend le moteur désigné par l'environnement ("" s'il n'y en a pas).
// DOCKER_CONTEXT=default revient au comportement normal : c'est le contexte par défaut du client docker.
func externalEngine(getenv func(string) string) string {
	if h := strings.TrimSpace(getenv("DOCKER_HOST")); h != "" {
		return h
	}
	if c := strings.TrimSpace(getenv("DOCKER_CONTEXT")); c != "" && !strings.EqualFold(c, "default") {
		return "contexte " + c
	}
	return ""
}

// externalEnv : environnement de l'utilisateur, intact. Seule exception : sans DOCKER_CONFIG ni
// DOCKER_CONTEXT, on pointe DOCKER_CONFIG sur la config d'Orvaen pour retrouver compose et buildx
// (un contexte, lui, est rangé dans la config de l'utilisateur : on n'y touche pas).
func externalEnv(environ []string, getenv func(string) string) []string {
	env := append([]string(nil), environ...)
	if strings.TrimSpace(getenv("DOCKER_CONFIG")) == "" && strings.TrimSpace(getenv("DOCKER_CONTEXT")) == "" {
		env = append(env, "DOCKER_CONFIG="+clientConfig())
	}
	return env
}

// runExternal lance la commande vers le moteur choisi par l'utilisateur.
func runExternal(args []string, target string) int {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		ui.Errorf("DOCKER_HOST / DOCKER_CONTEXT (%s) : client docker Windows indisponible sur cette machine.", target)
		return 1
	}
	if !clientInstalled() {
		ui.Info("Installation du client docker pour Windows (une seule fois)…")
		if err := installClient(); err != nil {
			ui.Errorf("client docker Windows non installé (%v) : impossible de joindre %s.", err, target)
			return 1
		}
	}
	if os.Getenv("ORVAEN_DOCKER_QUIET") == "" {
		ui.Info("Moteur : " + target + " (DOCKER_HOST / DOCKER_CONTEXT). Pour revenir au moteur d'Orvaen, vide ces variables.")
	}
	cmd := exec.Command(clientExe(), args...)
	cmd.Env = externalEnv(os.Environ(), os.Getenv)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return exitCode(cmd.Run())
}
