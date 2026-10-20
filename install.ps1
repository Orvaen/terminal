# Installe apt pour Windows dans le dossier de l'utilisateur et l'ajoute au PATH.
# Usage : place install.ps1 à côté de apt.exe, puis lance  powershell -ExecutionPolicy Bypass -File .\install.ps1
$ErrorActionPreference = 'Stop'

$src = Join-Path $PSScriptRoot 'apt.exe'
if (-not (Test-Path $src)) {
    throw "apt.exe est introuvable à côté de install.ps1. Télécharge une release ou compile-le (voir README)."
}

$dest = Join-Path $env:LOCALAPPDATA 'Programs\apt-windows'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Copy-Item $src (Join-Path $dest 'apt.exe') -Force
Copy-Item $src (Join-Path $dest 'apt-get.exe') -Force

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $userPath) { $userPath = '' }
$entries = $userPath -split ';' | Where-Object { $_ -ne '' }
if ($entries -notcontains $dest) {
    [Environment]::SetEnvironmentVariable('Path', (($entries + $dest) -join ';'), 'User')
    Write-Host "Ajouté au PATH : $dest"
}

Write-Host ''
Write-Host 'apt est installé. Ouvre un NOUVEAU terminal et tape :  apt help'
