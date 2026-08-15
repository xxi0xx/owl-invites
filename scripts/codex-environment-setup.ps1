$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "codex-env.ps1")

Set-Location $env:OWL_CODEX_REPO_ROOT

Write-Host ""
Write-Host "============================================"
Write-Host "Owl Invites - Codex environment setup"
Write-Host "============================================"

# --------------------------------------------------
# Required host tools
# --------------------------------------------------

$requiredCommands = @(
    "git",
    "go",
    "node",
    "npm",
    "gcc"
)

foreach ($command in $requiredCommands) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw @"
Required host dependency '$command' is missing.

Do NOT troubleshoot broadly inside a Codex task.

Run the human-only Windows bootstrap:

    powershell -ExecutionPolicy Bypass `
      -File scripts\codex-windows-bootstrap.ps1

Then restart Codex.
"@
    }
}

# --------------------------------------------------
# Toolchain validation
# --------------------------------------------------

Write-Host ""
Write-Host "== Toolchain =="

$goVersion = (& go env GOVERSION).Trim()

if ($LASTEXITCODE -ne 0) {
    throw "Unable to resolve the Go 1.26.5 toolchain."
}

if ($goVersion -ne "go1.26.5") {
    throw "Expected Go 1.26.5; got $goVersion."
}

Write-Host "Go:   $goVersion"

$nodeVersion = (& node --version).Trim()
$nodeMajor = (& node -p "process.versions.node.split('.')[0]").Trim()

if ($nodeMajor -ne "22") {
    throw "Expected Node.js 22; got $nodeVersion."
}

Write-Host "Node: $nodeVersion"
Write-Host "npm:  $((& npm --version).Trim())"

$gccVersion = (& gcc --version | Select-Object -First 1)

if ($LASTEXITCODE -ne 0) {
    throw "gcc exists but failed to execute."
}

Write-Host "GCC:  $gccVersion"

# --------------------------------------------------
# Backend dependencies
# --------------------------------------------------

Write-Host ""
Write-Host "== Go modules =="

& go mod download

if ($LASTEXITCODE -ne 0) {
    throw "go mod download failed."
}

# --------------------------------------------------
# Frontend dependencies
# --------------------------------------------------

Write-Host ""
Write-Host "== npm dependencies =="

Push-Location (Join-Path $env:OWL_CODEX_REPO_ROOT "web")

try {
    & npm ci

    if ($LASTEXITCODE -ne 0) {
        throw "npm ci failed."
    }

    Write-Host ""
    Write-Host "== Playwright Chromium =="

    & npx playwright install chromium

    if ($LASTEXITCODE -ne 0) {
        throw "Playwright Chromium installation failed."
    }
}
finally {
    Pop-Location
}

# --------------------------------------------------
# Development/security tooling
# --------------------------------------------------

Write-Host ""
Write-Host "== govulncheck =="

& go install `
    golang.org/x/vuln/cmd/govulncheck@v1.1.4

if ($LASTEXITCODE -ne 0) {
    throw "govulncheck installation failed."
}

Write-Host ""
Write-Host "== golangci-lint =="

& go install `
    github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

if ($LASTEXITCODE -ne 0) {
    throw "golangci-lint installation failed."
}

# --------------------------------------------------
# go:embed placeholder
# --------------------------------------------------

$frontendEmbed = Join-Path `
    $env:OWL_CODEX_REPO_ROOT `
    "internal\server\frontend"

if (-not (Test-Path $frontendEmbed)) {
    New-Item `
        -ItemType Directory `
        -Path $frontendEmbed `
        -Force | Out-Null
}

$embedIndex = Join-Path $frontendEmbed "index.html"

if (-not (Test-Path $embedIndex)) {
    New-Item `
        -ItemType File `
        -Path $embedIndex `
        -Force | Out-Null
}

# --------------------------------------------------
# Summary
# --------------------------------------------------

Write-Host ""
Write-Host "============================================"
Write-Host "Environment setup complete"
Write-Host "============================================"

Write-Host "Go:            $goVersion"
Write-Host "Node:          $nodeVersion"
Write-Host "GCC:           $gccVersion"
Write-Host "Tool bin:      $env:GOBIN"
Write-Host "Go mod cache:  $env:GOMODCACHE"
Write-Host "Go build cache:$env:GOCACHE"
Write-Host "npm cache:     $env:npm_config_cache"
Write-Host "Playwright:    $env:PLAYWRIGHT_BROWSERS_PATH"

Write-Host ""
Write-Host "Next:"
Write-Host ""
Write-Host "  powershell -NoProfile -ExecutionPolicy Bypass "
Write-Host "     -File scripts\codex-preflight.ps1"
Write-Host ""