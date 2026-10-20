# Compile apt.exe, terminal.exe, docker.exe et req.exe puis lance l'installation.
# Usage : powershell -ExecutionPolicy Bypass -File .\build.ps1
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

foreach ($name in 'apt', 'terminal', 'docker', 'req') {
    Write-Host "Compilation de $name.exe..."
    go build -trimpath -o "$name.exe" "./cmd/$name"
    if ($LASTEXITCODE -ne 0) { throw "La compilation de $name.exe a échoué." }
}

& (Join-Path $PSScriptRoot 'install.ps1')
