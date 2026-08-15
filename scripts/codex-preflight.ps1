param(
    [switch]$RequireCleanWorktree
)

$ErrorActionPreference = "Continue"

. (Join-Path $PSScriptRoot "codex-env.ps1")

# codex-env.ps1 must not modify the caller's error policy.
$ErrorActionPreference = "Continue"

Set-Location $env:OWL_CODEX_REPO_ROOT

$PassCount = 0
$WarnCount = 0
$FailCount = 0

function Pass {
    param([string]$Message)

    Write-Host "PASS  $Message" -ForegroundColor Green
    $script:PassCount++
}

function Warn {
    param([string]$Message)

    Write-Host "WARN  $Message" -ForegroundColor Yellow
    $script:WarnCount++
}

function Fail {
    param([string]$Message)

    Write-Host "FAIL  $Message" -ForegroundColor Red
    $script:FailCount++
}

Write-Host ""
Write-Host "============================================"
Write-Host "Owl Invites - Codex preflight"
Write-Host "============================================"
Write-Host ""

# --------------------------------------------------
# Git
# --------------------------------------------------

if (Get-Command git -ErrorAction SilentlyContinue) {
    & git rev-parse --show-toplevel *> $null

    if ($LASTEXITCODE -eq 0) {
        Pass "Git repository available"
    }
    else {
        Fail "Not inside a Git repository"
    }
}
else {
    Fail "Git missing"
}

if ($RequireCleanWorktree) {
    $status = & git status --porcelain

    if ([string]::IsNullOrWhiteSpace(($status -join ""))) {
        Pass "Worktree is clean"
    }
    else {
        Fail "Worktree is not clean"
    }
}

# --------------------------------------------------
# Go
# --------------------------------------------------

if (Get-Command go -ErrorAction SilentlyContinue) {
    $goVersion = (& go env GOVERSION 2>$null).Trim()

    if ($goVersion -eq "go1.26.6") {
        Pass "Go 1.26.6"
    }
    else {
        Fail "Expected Go 1.26.6; found '$goVersion'"
    }
}
else {
    Fail "Go executable missing"
}

# --------------------------------------------------
# GCC / CGO
# --------------------------------------------------

if (Get-Command gcc -ErrorAction SilentlyContinue) {
    Pass "gcc available"
}
else {
    Fail "gcc missing"
}

$cgoRoot = Join-Path `
    $env:OWL_CODEX_CACHE `
    ("preflight-cgo-" + [Guid]::NewGuid().ToString("N"))

try {
    New-Item `
        -ItemType Directory `
        -Path $cgoRoot `
        -Force | Out-Null

    $cgoFile = Join-Path $cgoRoot "main.go"
    $cgoExe = Join-Path $cgoRoot "cgo-check.exe"

    $cgoSource = @'
package main

/*
#include <stdlib.h>
*/
import "C"

func main() {}
'@

    # Write UTF-8 without BOM for maximum toolchain compatibility.
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)

    [System.IO.File]::WriteAllText(
        $cgoFile,
        $cgoSource,
        $utf8NoBom
    )

    $oldPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"

    try {
        $cgoOutput = @(
            & go build `
                -o $cgoExe `
                $cgoFile 2>&1
        )

        $cgoExit = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $oldPreference
    }

    if ($cgoExit -eq 0 -and (Test-Path $cgoExe)) {
        Pass "CGO native compilation works"
    }
    else {
        Fail "CGO compilation failed"

        foreach ($line in $cgoOutput) {
            Write-Host "      $line"
        }
    }
}
catch {
    Fail "CGO check errored: $($_.Exception.Message)"
}
finally {
    if (Test-Path $cgoRoot) {
        Remove-Item `
            -Recurse `
            -Force `
            $cgoRoot `
            -ErrorAction SilentlyContinue
    }
}
# --------------------------------------------------
# Node/npm
# --------------------------------------------------

if (Get-Command node -ErrorAction SilentlyContinue) {
    $nodeVersion = (& node --version).Trim()
    $nodeMajor = (& node -p "process.versions.node.split('.')[0]").Trim()

    if ($nodeMajor -eq "22") {
        Pass "Node.js 22 ($nodeVersion)"
    }
    else {
        Fail "Expected Node.js 22; found $nodeVersion"
    }
}
else {
    Fail "Node.js missing"
}

if (Get-Command npm -ErrorAction SilentlyContinue) {
    Pass "npm available"
}
else {
    Fail "npm missing"
}

# --------------------------------------------------
# npm dependency tree
# --------------------------------------------------

$webDir = Join-Path $env:OWL_CODEX_REPO_ROOT "web"

if (Test-Path (Join-Path $webDir "node_modules")) {
    Push-Location $webDir

    try {
        & npm ls --depth=0 *> $null

        if ($LASTEXITCODE -eq 0) {
            Pass "Frontend npm dependencies installed"
        }
        else {
            Fail "Frontend npm dependency tree incomplete"
        }
    }
    finally {
        Pop-Location
    }
}
else {
    Fail "web\node_modules missing; run environment setup"
}

# --------------------------------------------------
# Playwright Chromium
# --------------------------------------------------

if (Test-Path (Join-Path $webDir "node_modules")) {

    $playwrightCheckFile = Join-Path `
        $webDir `
        ".codex-playwright-preflight.mjs"

    $playwrightSource = @'
import { chromium } from "@playwright/test";

const browser = await chromium.launch({
    headless: true
});

await browser.close();
'@

    try {
        $utf8NoBom = New-Object System.Text.UTF8Encoding($false)

        [System.IO.File]::WriteAllText(
            $playwrightCheckFile,
            $playwrightSource,
            $utf8NoBom
        )

        Push-Location $webDir

        try {
            $oldPreference = $ErrorActionPreference
            $ErrorActionPreference = "Continue"

            try {
                $playwrightOutput = @(
                    & node $playwrightCheckFile 2>&1
                )

                $playwrightExit = $LASTEXITCODE
            }
            finally {
                $ErrorActionPreference = $oldPreference
            }
        }
        finally {
            Pop-Location
        }

        if ($playwrightExit -eq 0) {
            Pass "Playwright Chromium launches"
        }
        else {
            Fail "Playwright Chromium unavailable or cannot launch"

            foreach ($line in $playwrightOutput) {
                Write-Host "      $line"
            }
        }
    }
    catch {
        Fail "Playwright check errored: $($_.Exception.Message)"
    }
    finally {
        Remove-Item `
            $playwrightCheckFile `
            -Force `
            -ErrorAction SilentlyContinue
    }
}
# --------------------------------------------------
# Security/lint tools
# --------------------------------------------------

$govulncheck = Join-Path `
    $env:OWL_CODEX_TOOLS `
    "bin\govulncheck.exe"

if (Test-Path $govulncheck) {
    Pass "govulncheck 1.1.4 installed"
}
else {
    Fail "govulncheck missing"
}

$golangci = Join-Path `
    $env:OWL_CODEX_TOOLS `
    "bin\golangci-lint.exe"

if (Test-Path $golangci) {
    Pass "golangci-lint 2.12.2 installed"
}
else {
    Fail "golangci-lint missing"
}

# --------------------------------------------------
# Optional integration infrastructure
# --------------------------------------------------

if (Get-Command psql -ErrorAction SilentlyContinue) {
    Pass "PostgreSQL client available (optional)"
}
else {
    Warn "PostgreSQL client unavailable; GitHub Actions remains authoritative"
}

if (Get-Command docker -ErrorAction SilentlyContinue) {
    & docker info *> $null

    if ($LASTEXITCODE -eq 0) {
        Pass "Docker daemon available (optional)"
    }
    else {
        Warn "Docker CLI found but daemon unavailable; Docker is optional"
    }
}
else {
    Warn "Docker unavailable; Docker is optional"
}

# --------------------------------------------------
# Codex Security reminder
# --------------------------------------------------

Write-Host ""
Write-Host "NOTE  Deep Security Scan requires its own plugin"
Write-Host "      capability preflight after this shell preflight."
Write-Host ""

# --------------------------------------------------
# Result
# --------------------------------------------------

Write-Host "============================================"
Write-Host "PASS=$PassCount  WARN=$WarnCount  FAIL=$FailCount"
Write-Host "============================================"

if ($FailCount -gt 0) {
    Write-Host ""
    Write-Host "PRE-FLIGHT FAILED" -ForegroundColor Red
    Write-Host ""
    Write-Host "Report the exact failed prerequisite."
    Write-Host "Do not perform broad environment troubleshooting."
    exit 1
}

Write-Host ""
Write-Host "PRE-FLIGHT PASSED" -ForegroundColor Green
exit 0