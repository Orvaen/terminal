//go:build !windows

package docker

import "errors"

func startDetached(name string, args, env []string) error {
	return errors.New("disponible uniquement sous Windows")
}
