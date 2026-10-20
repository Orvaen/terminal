# Installe Orvaen Terminal (terminal.exe) et apt.exe pour l'utilisateur courant :
#  - copie dans %LOCALAPPDATA%\Programs\apt-windows et ajout au PATH
#  - raccourcis dans le menu Démarrer et sur le Bureau
#  - profil dans Windows Terminal
# Usage : place install.ps1 à côté des .exe, puis  powershell -ExecutionPolicy Bypass -File .\install.ps1
$ErrorActionPreference = 'Stop'
$AppName = 'Orvaen Terminal'

$aptSrc  = Join-Path $PSScriptRoot 'apt.exe'
$termSrc = Join-Path $PSScriptRoot 'terminal.exe'
foreach ($f in $aptSrc, $termSrc) {
    if (-not (Test-Path $f)) {
        throw "$(Split-Path $f -Leaf) est introuvable à côté de install.ps1. Lance build.ps1 ou télécharge une release (voir README)."
    }
}

# 1. Copie des exécutables
$dest = Join-Path $env:LOCALAPPDATA 'Programs\apt-windows'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
$termExe = Join-Path $dest 'terminal.exe'
Copy-Item $aptSrc  (Join-Path $dest 'apt.exe') -Force
Copy-Item $aptSrc  (Join-Path $dest 'apt-get.exe') -Force
Copy-Item $termSrc $termExe -Force
Write-Host "Copié dans $dest"

# 2. PATH utilisateur (avec réparation d'une entrée collée par l'ancienne version de l'installeur)
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $userPath) { $userPath = '' }
$entries = @()
foreach ($e in @($userPath -split ';')) {
    if ($e -eq '') { continue }
    if ($e -ne $dest -and $e.EndsWith($dest, [StringComparison]::OrdinalIgnoreCase)) {
        $e = $e.Substring(0, $e.Length - $dest.Length)
        Write-Host "PATH réparé : $e"
    }
    if ($e -ne '') { $entries += $e }
}
if ($entries -notcontains $dest) { $entries += $dest }
$newPath = $entries -join ';'
if ($newPath -ne $userPath) {
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Write-Host "PATH mis à jour : $dest"
}

# 3. Raccourcis menu Démarrer et Bureau
$wsh = New-Object -ComObject WScript.Shell
foreach ($folder in [Environment]::GetFolderPath('Programs'), [Environment]::GetFolderPath('Desktop')) {
    $lnk = $wsh.CreateShortcut((Join-Path $folder "$AppName.lnk"))
    $lnk.TargetPath       = $termExe
    $lnk.WorkingDirectory = $env:USERPROFILE
    $lnk.Description      = 'Terminal façon Linux avec apt et sudo'
    $lnk.Save()
}
Write-Host 'Raccourcis créés (menu Démarrer et Bureau)'

# 4. Profil Windows Terminal (fragment JSON, pris en compte au prochain lancement de Windows Terminal)
$fragDir = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows Terminal\Fragments\OrvaenTerminal'
New-Item -ItemType Directory -Force -Path $fragDir | Out-Null
$fragment = @{
    profiles = @(
        @{
            name              = $AppName
            commandline       = $termExe
            startingDirectory = '%USERPROFILE%'
        }
    )
} | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText((Join-Path $fragDir 'profile.json'), $fragment)
Write-Host 'Profil ajouté à Windows Terminal'

Write-Host ''
Write-Host "C'est installé ! Lancement de $AppName..."
Start-Process -FilePath $termExe -WorkingDirectory $env:USERPROFILE
