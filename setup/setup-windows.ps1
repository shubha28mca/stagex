# Requires PowerShell 5.1+ (built into Windows 10/11).
# One-shot setup for IIG StageX on Windows: installs Docker Desktop (via winget)
# if missing, waits for the Docker engine, then builds and starts the stack.
#
#   Right-click > "Run with PowerShell", or:  powershell -ExecutionPolicy Bypass -File setup\setup-windows.ps1

#Requires -Version 5.1
$ErrorActionPreference = 'Stop'

# Move to the repository root (this script lives in <repo>/setup).
$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

function Test-Command($name) { $null -ne (Get-Command $name -ErrorAction SilentlyContinue) }
function Test-DockerEngine { try { docker info *> $null; return $true } catch { return $false } }

Write-Host "== IIG StageX setup (Windows) ==" -ForegroundColor Cyan

# 1) Ensure Docker Desktop is installed.
if (-not (Test-Command docker)) {
    Write-Host "Docker not found. Installing Docker Desktop via winget..." -ForegroundColor Yellow
    if (-not (Test-Command winget)) {
        throw "winget is required. Install 'App Installer' from the Microsoft Store, then re-run this script."
    }
    winget install --id Docker.DockerDesktop -e --accept-source-agreements --accept-package-agreements
    # Make docker available in this session without a new terminal.
    $env:Path += ";C:\Program Files\Docker\Docker\resources\bin"
    Write-Host "Docker Desktop installed. If Windows asks to sign out/restart (WSL2), do that first." -ForegroundColor Yellow
}

# 2) Make sure the Docker engine is actually running (start Docker Desktop if not).
if (-not (Test-DockerEngine)) {
    $dd = "C:\Program Files\Docker\Docker\Docker Desktop.exe"
    if (Test-Path $dd) {
        Write-Host "Starting Docker Desktop..." -ForegroundColor Yellow
        Start-Process $dd
    }
    Write-Host "Waiting for the Docker engine to start (up to 3 minutes)..." -ForegroundColor Yellow
    $deadline = (Get-Date).AddMinutes(3)
    while (-not (Test-DockerEngine)) {
        if ((Get-Date) -gt $deadline) {
            throw "Docker engine did not start. Open Docker Desktop, wait until it says 'Running', then re-run this script."
        }
        Start-Sleep -Seconds 3
    }
}
Write-Host "Docker engine is running." -ForegroundColor Green

# 3) Build and start the whole stack.
Write-Host "Building and starting containers (first run downloads images, be patient)..." -ForegroundColor Cyan
docker compose up --build -d

Write-Host ""
Write-Host "StageX is up:" -ForegroundColor Green
Write-Host "  Participant app : http://localhost:5173"
Write-Host "  Admin console   : http://localhost:5174"
Write-Host "  Participant API : http://localhost:8080/api/health"
Write-Host "  Admin API       : http://localhost:8081/admin/health"
Write-Host ""
Write-Host "Admin demo logins:" -ForegroundColor Cyan
Write-Host "  Operational Admin : ops@stagex.test / Ops@12345"
Write-Host "  Event Admin       : event@stagex.test / Event@12345"
Write-Host ""
Write-Host "Stop it later with:  docker compose down" -ForegroundColor DarkGray
