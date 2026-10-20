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
