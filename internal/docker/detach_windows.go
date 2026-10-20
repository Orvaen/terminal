//go:build windows

package docker

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow         = 0x08000000
	createBreakawayFromJob = 0x01000000
)

// startDetached lance un processus sans fenêtre, indépendant de la console du terminal :
// fermer le terminal n'arrête pas le moteur Docker.
func startDetached(name string, args, env []string) error {
	start := func(flags uint32) error {
		cmd := exec.Command(name, args...)
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: flags | createNoWindow | syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	// On tente de sortir du « job » de la fenêtre (sinon sa fermeture tuerait le moteur) ;
	// si ce n'est pas autorisé, on démarre normalement.
	if err := start(createBreakawayFromJob); err == nil {
		return nil
	}
	return start(0)
}
