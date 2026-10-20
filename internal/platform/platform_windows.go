//go:build windows

// Package platform regroupe ce qui dépend du système : droits admin, élévation et console.
package platform

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

const (
	tokenElevation                  = 20     // TOKEN_INFORMATION_CLASS.TokenElevation
	enableVirtualTerminalProcessing = 0x0004 // mode console pour les séquences ANSI
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

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

// RunSelfElevated relance l'exécutable courant en administrateur (fenêtre UAC),
// attend la fin et renvoie son code de sortie.
func RunSelfElevated(args ...string) int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = psQuote(a)
	}
	script := "$p = Start-Process -FilePath " + psQuote(exe) +
		" -ArgumentList " + strings.Join(quoted, ",") +
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
