package apt

// MSVC, le compilateur C/C++ de Microsoft, sans jamais ouvrir Visual Studio Installer :
//
//   - build-essential : les Build Tools de Visual Studio, avec la charge de travail C++ et les
//     bibliothèques « Spectre-mitigated » déjà cochées ;
//   - msvc-spectre : ajoute ces bibliothèques à un MSVC déjà installé, en pilotant l'installeur
//     de Visual Studio en ligne de commande.
//
// Les bibliothèques Spectre-mitigated sont les versions du runtime C et de la STL compilées avec
// la protection /Qspectre. Un projet qui l'active (VS Code et ses modules natifs, par exemple)
// échoue sans elles : « MSB8040: Spectre-mitigated libraries are required for this project ».

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Orvaen/terminal/internal/ui"
)

// buildEssentialNames : les noms qui installent le compilateur MSVC.
var buildEssentialNames = map[string]bool{
	"build-essential": true, "msvc": true, "buildtools": true, "vs-buildtools": true,
	"visual-studio-build-tools": true, "visualcpp-build-tools": true,
}

// spectreNames : les noms qui ajoutent les bibliothèques Spectre-mitigated.
var spectreNames = map[string]bool{
	"msvc-spectre": true, "spectre": true, "vc-spectre": true, "spectre-libs": true,
}

const (
	msvcID           = "Microsoft.VisualStudio.BuildTools"
	vcToolsComponent = "Microsoft.VisualStudio.Component.VC.Tools.x86.x64"
	vcWorkload       = "Microsoft.VisualStudio.Workload.VCTools"
)

// spectreComponents : les bibliothèques Spectre-mitigated pour l'architecture du PC.
func spectreComponents() []string {
	comps := []string{"Microsoft.VisualStudio.Component.VC.Runtimes.x86.x64.Spectre"}
	if runtime.GOARCH == "arm64" {
		comps = append(comps, "Microsoft.VisualStudio.Component.VC.Runtimes.ARM64.Spectre")
	}
	return comps
}

func vsInstallerDir() string {
	base := os.Getenv("ProgramFiles(x86)")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Microsoft Visual Studio", "Installer")
}

// Points d'injection, remplacés dans les tests.
var (
	// vswhere renvoie le dossier de la dernière installation de Visual Studio qui a tous ces composants.
	vswhere = func(requires ...string) string {
		dir := vsInstallerDir()
		if dir == "" {
			return ""
		}
		args := []string{"-latest", "-products", "*", "-property", "installationPath"}
		for _, r := range requires {
			args = append(args, "-requires", r)
		}
		out, err := exec.Command(filepath.Join(dir, "vswhere.exe"), args...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	}
	// runVSInstaller lance l'installeur de Visual Studio (il demande lui-même l'élévation) et attend la fin.
	runVSInstaller = func(args ...string) int {
		dir := vsInstallerDir()
		if dir == "" {
			ui.Errorf("installeur de Visual Studio introuvable")
			return 1
		}
		cmd := exec.Command(filepath.Join(dir, "setup.exe"), args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			ui.Errorf("impossible de lancer l'installeur de Visual Studio : %v", err)
			return 1
		}
		return 0
	}
)

// msvcInstalled : un Visual Studio ou des Build Tools avec le compilateur C++.
var msvcInstalled = func() bool { return vswhere(vcToolsComponent) != "" }

func spectreInstalled() bool {
	return vswhere(append([]string{vcToolsComponent}, spectreComponents()...)...) != ""
}

// installBuildEssential installe les Build Tools avec la charge de travail C++ et les bibliothèques
// Spectre-mitigated, ce qu'un simple « winget install » ne coche pas.
func installBuildEssential() int {
	if msvcInstalled() {
		ui.Info("MSVC est déjà installé.")
		if !spectreInstalled() {
			return installSpectre()
		}
		return 0
	}
	ui.Info("build-essential → " + msvcID + " : compilateur C++, SDK Windows, CMake et bibliothèques Spectre-mitigated")
	ui.Info("Le téléchargement fait plusieurs Go ; Windows demandera une confirmation (UAC).")
	override := "--wait --passive --norestart --add " + vcWorkload + " --includeRecommended"
	for _, c := range spectreComponents() {
		override += " --add " + c
	}
	code := winget("install", "--id", msvcID, "--exact", acceptSource, acceptPackage, "--override", override)
	if code == 0 {
		ui.Info("cl.exe s'utilise depuis « Developer PowerShell for VS » ; CMake et node-gyp trouvent MSVC tout seuls.")
		ui.Info("Pour gcc dans le terminal : apt install gcc")
	}
	return code
}

// installSpectre ajoute les bibliothèques Spectre-mitigated au MSVC installé (ou installe tout).
func installSpectre() int {
	if spectreInstalled() {
		ui.Info("Les bibliothèques Spectre-mitigated de MSVC sont déjà installées.")
		return 0
	}
	path := vswhere(vcToolsComponent)
	if path == "" {
		ui.Info("MSVC n'est pas installé : installation complète (build-essential).")
		return installBuildEssential()
	}
	ui.Info("Ajout des bibliothèques Spectre-mitigated à " + path)
	ui.Info("L'installeur de Visual Studio travaille seul ; Windows demandera une confirmation (UAC).")
	args := []string{"modify", "--installPath", path}
	for _, c := range spectreComponents() {
		args = append(args, "--add", c)
	}
	args = append(args, "--passive", "--norestart")
	code := runVSInstaller(args...)
	switch {
	case code == 0 || code == msiRebootRequired:
		ui.Info("Bibliothèques Spectre-mitigated installées.")
		return 0
	case code == 1 && !spectreInstalled():
		ui.Errorf("l'installeur de Visual Studio a échoué. Ferme Visual Studio s'il est ouvert, puis relance : apt install msvc-spectre")
	default:
		ui.Errorf("l'installeur de Visual Studio a échoué (code %d)", code)
	}
	return code
}

// isMSB8040 : la sortie d'une compilation réclame les bibliothèques Spectre-mitigated.
func isMSB8040(output string) bool {
	return strings.Contains(output, "MSB8040")
}
