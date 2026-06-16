#!/usr/bin/env pwsh
# VinoLlama build script for Windows
# Usage: ./scripts/build.ps1 [-SkipTests] [-SkipFrontend] [-SkipDesktop] [-Clean]

param(
    [switch]$SkipTests,
    [switch]$SkipFrontend,
    [switch]$SkipDesktop,
    [switch]$Clean
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $PSCommandPath)

Write-Host "=== VinoLlama Build ===" -ForegroundColor Cyan
Write-Host ""

# Clean
if ($Clean) {
    Write-Host "[clean] Removing build artifacts..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue "$Root\desktop\build\bin"
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue "$Root\desktop\frontend\dist"
    Write-Host "[clean] Done." -ForegroundColor Green
}

# Backend tests
if (-not $SkipTests) {
    Write-Host "[test] Running Go tests..." -ForegroundColor Yellow
    Push-Location $Root
    go test ./...
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }
    Pop-Location
    Write-Host "[test] Go tests passed." -ForegroundColor Green
    Write-Host ""
}

# Frontend checks
if (-not $SkipFrontend) {
    Write-Host "[frontend] Installing dependencies..." -ForegroundColor Yellow
    Push-Location "$Root\desktop\frontend"
    npm install
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }

    Write-Host "[frontend] Running typecheck..." -ForegroundColor Yellow
    npm run typecheck
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }

    Write-Host "[frontend] Running tests..." -ForegroundColor Yellow
    npm test
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }

    Write-Host "[frontend] Building frontend..." -ForegroundColor Yellow
    npm run build
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }
    Pop-Location
    Write-Host "[frontend] Frontend checks passed." -ForegroundColor Green
    Write-Host ""
}

# Desktop build (Wails)
if (-not $SkipDesktop) {
    Write-Host "[desktop] Building Wails desktop app..." -ForegroundColor Yellow
    Push-Location "$Root\desktop"
    wails build
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit 1 }
    Pop-Location
    Write-Host "[desktop] Desktop build complete." -ForegroundColor Green
    Write-Host ""

    $ExePath = "$Root\desktop\build\bin\VinoLlama.exe"
    if (Test-Path $ExePath) {
        $Size = (Get-Item $ExePath).Length / 1MB
        Write-Host "Output: $ExePath ($([math]::Round($Size, 1)) MB)" -ForegroundColor Cyan
    }
}

Write-Host ""
Write-Host "=== Build Complete ===" -ForegroundColor Green
