package apt

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"runtime"

	"github.com/Orvaen/terminal/internal/platform"
	"github.com/Orvaen/terminal/internal/ui"
)

// Code de sortie utilisé par le script quand un redémarrage est nécessaire (ERROR_SUCCESS_REBOOT_REQUIRED).
const rebootRequired = 3010

// wuScript passe par l'API COM de Windows Update : aucun module PowerShell à installer.
const wuScript = `
$ErrorActionPreference = 'Stop'
$session  = New-Object -ComObject Microsoft.Update.Session
$searcher = $session.CreateUpdateSearcher()
Write-Host 'Recherche des mises à jour Windows...'
$result = $searcher.Search("IsInstalled=0 and IsHidden=0")
if ($result.Updates.Count -eq 0) { Write-Host 'Windows est à jour.'; exit 0 }

$updates = New-Object -ComObject Microsoft.Update.UpdateColl
foreach ($u in $result.Updates) {
    if (-not $u.EulaAccepted) { $u.AcceptEula() }
    [void]$updates.Add($u)
    Write-Host (' - ' + $u.Title)
}

Write-Host 'Téléchargement...'
$downloader = $session.CreateUpdateDownloader()
$downloader.Updates = $updates
[void]$downloader.Download()

Write-Host 'Installation...'
$installer = $session.CreateUpdateInstaller()
$installer.Updates = $updates
$res = $installer.Install()

switch ($res.ResultCode) {
    2 { Write-Host 'Mises à jour installées.' }
    3 { Write-Host 'Mises à jour installées, avec des erreurs.' }
    4 { Write-Host 'Échec de l''installation.'; exit 1 }
    5 { Write-Host 'Installation annulée.'; exit 1 }
}
if ($res.RebootRequired) { exit 3010 }
exit 0
`

func windowsUpdate(pause bool) int {
	if pause {
		defer waitForEnter()
	}
	if runtime.GOOS != "windows" {
		ui.Errorf("Windows Update n'est disponible que sous Windows.")
		return 1
	}
	if !platform.IsElevated() {
		ui.Errorf("les mises à jour Windows demandent les droits admin : lance « sudo apt full-upgrade » ou un terminal administrateur.")
		return 1
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", wuScript)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			ui.Errorf("impossible de lancer PowerShell : %v", err)
			return 1
		}
		code = ee.ExitCode()
	}
	if code == rebootRequired {
		ui.Warn("Un redémarrage est nécessaire pour terminer l'installation.")
		return 0
	}
	return code
}

func waitForEnter() {
	ui.Info("Appuie sur Entrée pour fermer cette fenêtre.")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
