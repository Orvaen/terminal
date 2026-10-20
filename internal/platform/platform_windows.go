//go:build windows

// Package platform regroupe ce qui dépend du système : droits admin, élévation et console.
package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	tokenElevation                  = 20     // TOKEN_INFORMATION_CLASS.TokenElevation
	enableVirtualTerminalProcessing = 0x0004 // mode console pour les séquences ANSI
	fileAttributeHidden             = 0x2
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

// IsElevated indique si le processus tourne avec les droits administrateur.
func IsElevated() bool {
	tok, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer tok.Close()

	var elevation uint32
	var n uint32
	err = syscall.GetTokenInformation(tok, tokenElevation,
		(*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &n)
	return err == nil && elevation != 0
}

// EnableVirtualTerminal active les couleurs ANSI dans la console. Renvoie false si c'est impossible.
func EnableVirtualTerminal() bool {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false // sortie redirigée vers un fichier ou un pipe : pas de couleur
	}
	r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}

const (
	enableProcessedInput       = 0x0001
	enableLineInput            = 0x0002
	enableEchoInput            = 0x0004
	enableVirtualTerminalInput = 0x0200
)

// EnableRawInput passe la console en saisie touche par touche (avec séquences VT pour les flèches).
// Renvoie la fonction qui rétablit le mode d'origine, ou ok=false si l'entrée n'est pas une console.
func EnableRawInput() (restore func(), ok bool) {
	h, err := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	if err != nil {
		return nil, false
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return nil, false
	}
	raw := mode&^(enableProcessedInput|enableLineInput|enableEchoInput) | enableVirtualTerminalInput
	if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(raw)); r == 0 {
		return nil, false
	}
	return func() { procSetConsoleMode.Call(uintptr(h), uintptr(mode)) }, true
}

// TerminalWidth renvoie la largeur visible de la console (80 par défaut).
func TerminalWidth() int {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return 80
	}
	var info consoleScreenBufferInfo
	if r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&info))); r == 0 {
		return 80
	}
	if w := int(info.Window.Right-info.Window.Left) + 1; w >= 20 {
		return w
	}
	return 80
}

// TerminalHeight renvoie la hauteur visible de la console (24 par défaut).
func TerminalHeight() int {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return 24
	}
	var info consoleScreenBufferInfo
	if r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&info))); r == 0 {
		return 24
	}
	if n := int(info.Window.Bottom-info.Window.Top) + 1; n >= 5 {
		return n
	}
	return 24
}

var (
	procGetUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
	procLCIDToLocaleName         = kernel32.NewProc("LCIDToLocaleName")
)

// Locale : la langue d'affichage de Windows au format de LANG (« fr-FR » → « fr_FR »), « C » si inconnue.
// La langue d'affichage (celle des menus) passe avant le format régional (dates, nombres), qui peut
// être réglé sur un autre pays : c'est elle que LANG désigne pour les messages des programmes.
func Locale() string {
	buf := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	name := ""
	if lang, _, _ := procGetUserDefaultUILanguage.Call(); lang != 0 {
		if r, _, _ := procLCIDToLocaleName.Call(lang, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0); r != 0 {
			name = syscall.UTF16ToString(buf)
		}
	}
	if name == "" {
		if r, _, _ := procGetUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r != 0 {
			name = syscall.UTF16ToString(buf)
		}
	}
	if name == "" {
		return "C"
	}
	return strings.ReplaceAll(name, "-", "_")
}

// IsHidden indique si un fichier porte l'attribut « caché » de Windows.
func IsHidden(info fs.FileInfo) bool {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&fileAttributeHidden != 0
	}
	return false
}

// NativeSudoInline indique si le sudo intégré à Windows 11 est activé en mode « dans la même fenêtre ».
func NativeSudoInline() bool {
	if _, err := exec.LookPath("sudo.exe"); err != nil {
		return false
	}
	out, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Sudo`, "/v", "Enabled").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && strings.EqualFold(fields[0], "Enabled") {
			mode, err := strconv.ParseUint(fields[2], 0, 32)
			// 1 = nouvelle fenêtre, 2 = entrée désactivée, 3 = normal (dans la même fenêtre)
			return err == nil && (mode == 2 || mode == 3)
		}
	}
	return false
}

// RunSelfElevated relance l'exécutable courant en administrateur (fenêtre UAC),
// attend la fin et renvoie son code de sortie.
func RunSelfElevated(args ...string) int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}
	// On construit nous-mêmes la ligne d'arguments : Start-Process ne protège pas les espaces.
	escaped := make([]string, len(args))
	for i, a := range args {
		escaped[i] = syscall.EscapeArg(a)
	}
	script := "$p = Start-Process -FilePath " + psQuote(exe) +
		" -ArgumentList " + psQuote(strings.Join(escaped, " ")) +
		" -Verb RunAs -Wait -PassThru; exit $p.ExitCode"

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}

// psQuote entoure une chaîne de guillemets simples PowerShell.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// KillTree arrête un processus et tous ceux qu'il a lancés (npm → node, cmd → …).
func KillTree(p *os.Process) error {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(p.Pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return p.Kill()
	}
	return nil
}

// RegistryPathDirs lit les dossiers du PATH utilisateur et système enregistrés dans le registre.
// Un logiciel installé pendant que le terminal est ouvert y figure, alors que le PATH du processus
// en cours ne le voit pas encore.
func RegistryPathDirs() []string {
	var dirs []string
	for _, scope := range []string{"user", "system"} {
		for _, e := range RegistryPath(scope) {
			dirs = append(dirs, e.Dir)
		}
	}
	return dirs
}

// PathEntry : un dossier du PATH enregistré dans Windows, tel qu'écrit (%SystemRoot%\…) et développé.
type PathEntry struct {
	Raw string
	Dir string
}

var pathKeys = map[string]string{
	"user":   `HKCU\Environment`,
	"system": `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
}

// RegistryPath lit le PATH « user » ou « system » tel qu'il est enregistré (sans développer les
// variables : « reg query » affiche la valeur brute).
func RegistryPath(scope string) []PathEntry {
	cmd := exec.Command("reg", "query", pathKeys[scope], "/v", "Path")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var entries []PathEntry
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.EqualFold(fields[0], "Path") {
			continue
		}
		// « Path    REG_EXPAND_SZ    valeur avec espaces »
		idx := strings.Index(line, fields[1]) + len(fields[1])
		for _, raw := range strings.Split(strings.TrimSpace(line[idx:]), ";") {
			if raw = strings.TrimSpace(raw); raw != "" {
				entries = append(entries, PathEntry{Raw: raw, Dir: expandPercent(raw)})
			}
		}
	}
	return entries
}

// WritePath remplace le PATH « user » ou « system » (droits administrateur pour system) par ces
// entrées brutes. La valeur reste de type REG_EXPAND_SZ, pour garder %SystemRoot% & co, et Windows
// est prévenu du changement (les fenêtres ouvertes ensuite le voient).
func WritePath(scope string, raw []string) error {
	key := map[string]string{
		"user":   `HKCU:\Environment`,
		"system": `HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
	}[scope]
	target := map[string]string{"user": "User", "system": "Machine"}[scope]
	script := `$ErrorActionPreference = 'Stop'; ` +
		`Set-ItemProperty -Path ` + psQuote(key) + ` -Name Path -Type ExpandString -Value ` + psQuote(strings.Join(raw, ";")) + `; ` +
		// Une variable temporaire posée puis retirée fait diffuser le changement à Windows (WM_SETTINGCHANGE).
		`[Environment]::SetEnvironmentVariable('ORVAEN_PATH_REFRESH', '1', '` + target + `'); ` +
		`[Environment]::SetEnvironmentVariable('ORVAEN_PATH_REFRESH', $null, '` + target + `')`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v : %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// expandPercent développe les %VARIABLES% de Windows.
func expandPercent(s string) string {
	for {
		i := strings.Index(s, "%")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+1:], "%")
		if j < 0 {
			return s
		}
		name := s[i+1 : i+1+j]
		s = s[:i] + os.Getenv(name) + s[i+2+j:]
	}
}

var (
	procGetConsoleCP       = kernel32.NewProc("GetConsoleCP")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

const cpUTF8 = 65001

// EnableUTF8Console passe la console en UTF-8 (page de code 65001), pour que les programmes
// lancés (winget, git, node, Coreutils…) affichent correctement les accents : sans ça, la console
// garde la page de code OEM (850, 437…) et « à » s'affiche « ├á ».
// Renvoie la fonction qui rétablit les pages de code d'origine, à appeler avant de quitter.
func EnableUTF8Console() (restore func()) {
	noop := func() {}
	if os.Getenv("ORVAEN_NO_UTF8") != "" {
		return noop
	}
	in, _, _ := procGetConsoleCP.Call()
	out, _, _ := procGetConsoleOutputCP.Call()
	if in == 0 && out == 0 {
		return noop // pas de console (sortie redirigée, service…)
	}
	if out != cpUTF8 {
		procSetConsoleOutputCP.Call(cpUTF8)
	}
	if in != 0 && in != cpUTF8 {
		procSetConsoleCP.Call(cpUTF8)
	}
	return func() {
		if out != 0 && out != cpUTF8 {
			procSetConsoleOutputCP.Call(out)
		}
		if in != 0 && in != cpUTF8 {
			procSetConsoleCP.Call(in)
		}
	}
}

// AddUserPath ajoute dir à la fin du PATH de l'utilisateur, pour les prochaines fenêtres.
func AddUserPath(dir string) error { return editUserPath(dir, true) }

// RemoveUserPath retire dir du PATH de l'utilisateur.
func RemoveUserPath(dir string) error { return editUserPath(dir, false) }

func editUserPath(dir string, add bool) error {
	var raw []string
	for _, e := range RegistryPath("user") {
		if !SamePath(e.Dir, dir) {
			raw = append(raw, e.Raw)
		}
	}
	if add {
		raw = append(raw, dir)
	}
	return WritePath("user", raw)
}

// FindInstalledExe cherche name.exe dans le dossier des programmes installés (ceux de « Applications
// installées », d'après le registre), pour un installeur qui n'a pas ajouté son dossier au PATH.
// Renvoie le dossier qui le contient, ou "".
func FindInstalledExe(name string) string {
	script := `$n = ` + psQuote(name+".exe") + `; ` +
		`$keys = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*',` +
		`'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*',` +
		`'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'; ` +
		`foreach ($k in Get-ItemProperty $keys -ErrorAction SilentlyContinue) { ` +
		`$l = $k.InstallLocation; if (-not $l) { continue }; $l = $l.Trim('"'); ` +
		`foreach ($d in @($l, (Join-Path $l 'bin'))) { if (Test-Path (Join-Path $d $n)) { $d; exit } } }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
