//go:build windows

// Package platform regroupe ce qui dépend du système : droits admin, élévation et console.
package platform

import (
	"errors"
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
	for _, key := range []string{
		`HKCU\Environment`,
		`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
	} {
		cmd := exec.Command("reg", "query", key, "/v", "Path")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 || !strings.EqualFold(fields[0], "Path") {
				continue
			}
			// « Path    REG_EXPAND_SZ    valeur avec espaces »
			idx := strings.Index(line, fields[1]) + len(fields[1])
			for _, d := range strings.Split(strings.TrimSpace(line[idx:]), ";") {
				if d = strings.TrimSpace(expandPercent(d)); d != "" {
					dirs = append(dirs, d)
				}
			}
		}
	}
	return dirs
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

// AddUserPath ajoute dir au PATH de l'utilisateur (registre), pour les prochaines fenêtres.
func AddUserPath(dir string) error { return editUserPath(dir, true) }

// RemoveUserPath retire dir du PATH de l'utilisateur.
func RemoveUserPath(dir string) error { return editUserPath(dir, false) }

func editUserPath(dir string, add bool) error {
	op := `$e = $e | Where-Object { $_ -and $_.TrimEnd('\') -ine $d.TrimEnd('\') }`
	if add {
		op += `; $e += $d`
	}
	script := `$d = ` + psQuote(dir) + `; $p = [Environment]::GetEnvironmentVariable('Path', 'User'); ` +
		`if (-not $p) { $p = '' }; $e = @($p -split ';'); ` + op + `; ` +
		`[Environment]::SetEnvironmentVariable('Path', ($e -join ';'), 'User')`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
