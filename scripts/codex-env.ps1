$ErrorActionPreference = "Stop"

$RepoRoot = (
    Resolve-Path (Join-Path $PSScriptRoot "..")
).Path

$CodexTools = Join-Path $RepoRoot ".codex-tools"
$CodexCache = Join-Path $RepoRoot ".codex-cache"

$ToolBin = Join-Path $CodexTools "bin"
$PlaywrightBrowsers = Join-Path $CodexTools "playwright"

$GoModuleCache = Join-Path $CodexCache "gomod"
$GoBuildCache = Join-Path $CodexCache "gobuild"
$NpmCache = Join-Path $CodexCache "npm"

$MsysGcc = "C:\msys64\ucrt64\bin"

$directories = @(
    $CodexTools,
    $CodexCache,
    $ToolBin,
    $PlaywrightBrowsers,
    $GoModuleCache,
    $GoBuildCache,
    $NpmCache
)

foreach ($directory in $directories) {
    if (-not (Test-Path $directory)) {
        New-Item `
            -ItemType Directory `
            -Path $directory `
            -Force | Out-Null
    }
}

# Canonical Owl Invites Go toolchain.
$env:GOTOOLCHAIN = "go1.26.6"
$env:CGO_ENABLED = "1"

# Keep Go artifacts inside the workspace so Codex sandbox
# identities do not need to rediscover/redownload them.
$env:GOBIN = $ToolBin
$env:GOMODCACHE = $GoModuleCache
$env:GOCACHE = $GoBuildCache

# Same idea for npm and Playwright.
$env:npm_config_cache = $NpmCache
$env:PLAYWRIGHT_BROWSERS_PATH = $PlaywrightBrowsers

# Make repo-local Go tools immediately callable.
if ($env:Path -notlike "*$ToolBin*") {
    $env:Path = "$ToolBin;$env:Path"
}

# Native Windows CGO support.
if (Test-Path $MsysGcc) {
    if ($env:Path -notlike "*$MsysGcc*") {
        $env:Path = "$MsysGcc;$env:Path"
    }
}

$env:OWL_CODEX_REPO_ROOT = $RepoRoot
$env:OWL_CODEX_TOOLS = $CodexTools
$env:OWL_CODEX_CACHE = $CodexCache