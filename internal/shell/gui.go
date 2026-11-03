package shell

import (
	"debug/pe"
	"strings"
)

// isGUIApp : vrai pour un .exe Windows « fenêtré » (VS Code, Notepad, Explorer…).
// Comme cmd et PowerShell, le terminal ne les attend pas : il rend la main tout de suite,
// et fermer le terminal ne les ferme pas.
func isGUIApp(path string) bool {
	if !strings.EqualFold(extOf(path), ".exe") {
		return false
	}
	f, err := pe.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return h.Subsystem == pe.IMAGE_SUBSYSTEM_WINDOWS_GUI
	case *pe.OptionalHeader32:
		return h.Subsystem == pe.IMAGE_SUBSYSTEM_WINDOWS_GUI
	}
	return false
}

func extOf(path string) string {
	i := strings.LastIndexAny(path, `.\/`)
	if i < 0 || path[i] != '.' {
		return ""
	}
	return path[i:]
}
