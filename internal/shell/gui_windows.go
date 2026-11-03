//go:build windows

package shell

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess        = 0x00000008
	createBreakawayFromJob = 0x01000000
)

// startDetached lance un programme fenêtré hors de la console et du « job » du terminal :
// fermer le terminal ne le tue pas.
func startDetached(cmd *exec.Cmd) error {
	start := func(flags uint32) error {
		c := exec.Command(cmd.Path, cmd.Args[1:]...)
		c.Env, c.Dir = cmd.Env, cmd.Dir
		c.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: flags | detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := c.Start(); err != nil {
			return err
		}
		return c.Process.Release()
	}
	// Sortir du job n'est pas toujours permis : on retente sans.
	if err := start(createBreakawayFromJob); err == nil {
		return nil
	}
	return start(0)
}
