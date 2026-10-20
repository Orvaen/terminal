package apt

import (
	"errors"
	"os"
	"os/exec"

	"github.com/Orvaen/terminal/internal/ui"
)

// Code renvoyé par winget quand aucune mise à jour n'est applicable (APPINSTALLER_CLI_ERROR_UPDATE_NOT_APPLICABLE).
const wingetUpdateNotApplicable = 0x8A15002B

var (
	acceptSource  = "--accept-source-agreements"
	acceptPackage = "--accept-package-agreements"
)

// Points d'injection, remplacés dans les tests.
var (
	winget          = runWinget
	wingetAvailable = func() bool { _, err := exec.LookPath("winget"); return err == nil }
)

func runWinget(args ...string) int {
	cmd := exec.Command("winget", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		ui.Errorf("impossible de lancer winget : %v", err)
		return 1
	}
	return 0
}

// target construit la partie « quel paquet » d'une commande winget.
func target(name string) []string {
	if id, exact := Resolve(name); exact {
		return []string{"--id", id, "--exact"}
	}
	return []string{name}
}

func isNotApplicable(code int) bool {
	return uint32(code) == wingetUpdateNotApplicable
}
