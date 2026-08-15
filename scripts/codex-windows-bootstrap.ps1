#requires -RunAsAdministrator

$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "============================================"
Write-Host "Owl Invites - Windows Codex host bootstrap"
Write-Host "============================================"
Write-Host ""

function Refresh-Path {
    $machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $user = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machine;$user"
}

function Invoke-WingetInstall {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Id
    )

    Write-Host ""
    Write-Host "Installing/checking $Id..."

    & winget install `
        --id $Id `
        --exact `
        --accept-package-agreements `
        --accept-source-agreements `
        --silent

    # winget may return non-zero when an equivalent/current package
    # is already installed, so refresh PATH and verify later.
    Refresh-Path
}

if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    throw "winget is required. Install/update Windows App Installer first."
}

Write-Host "Windows Package Manager:"
winget --version

# --------------------------------------------------
# Base development tools
# --------------------------------------------------

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Invoke-WingetInstall "Git.Git"
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Invoke-WingetInstall "GoLang.Go"
}

# --------------------------------------------------
# Node.js via Volta
# --------------------------------------------------

if (-not (Get-Command volta -ErrorAction SilentlyContinue)) {
    Invoke-WingetInstall "Volta.Volta"
}

Refresh-Path

if (-not (Get-Command volta -ErrorAction SilentlyContinue)) {
    throw "Volta is not available after installation."
}

Write-Host ""
Write-Host "Installing canonical Owl Invites Node version via Volta..."

& volta install node@22.23.2

if ($LASTEXITCODE -ne 0) {
    throw "Failed to install Node 22.23.2 through Volta."
}

$nodeVersion = (& node --version).Trim()

if ($nodeVersion -ne "v22.23.2") {
    throw "Expected Node v22.23.2 after Volta setup; got $nodeVersion."
}

Write-Host "Node: $nodeVersion"

if ($needsNode22) {
    Invoke-WingetInstall "OpenJS.NodeJS.22"
}

# --------------------------------------------------
# Native Windows CGO compiler
# --------------------------------------------------

$msysRoot = "C:\msys64"
$msysBash = Join-Path $msysRoot "usr\bin\bash.exe"
$gccDir = Join-Path $msysRoot "ucrt64\bin"
$gccExe = Join-Path $gccDir "gcc.exe"

if (-not (Test-Path $msysBash)) {
    Invoke-WingetInstall "MSYS2.MSYS2"
}

if (-not (Test-Path $msysBash)) {
    throw "MSYS2 was not found at $msysRoot after installation."
}

Write-Host ""
Write-Host "Updating MSYS2..."

& $msysBash -lc "pacman -Syu --noconfirm"

if ($LASTEXITCODE -ne 0) {
    Write-Host "Running MSYS2 update a second time..."
    & $msysBash -lc "pacman -Syu --noconfirm"
}

Write-Host ""
Write-Host "Installing UCRT64 GCC..."

& $msysBash -lc `
    "pacman -S --needed --noconfirm mingw-w64-ucrt-x86_64-gcc"

if ($LASTEXITCODE -ne 0) {
    throw "Failed to install MSYS2 UCRT64 GCC."
}

if (-not (Test-Path $gccExe)) {
    throw "gcc.exe was not found at $gccExe."
}

# --------------------------------------------------
# Put GCC on the normal Windows PATH
# --------------------------------------------------

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")

if ([string]::IsNullOrWhiteSpace($userPath)) {
    $userPath = ""
}

$pathEntries = $userPath -split ";" |
    Where-Object { -not [string]::IsNullOrWhiteSpace($_) }

if ($pathEntries -notcontains $gccDir) {
    if ([string]::IsNullOrWhiteSpace($userPath)) {
        $newUserPath = $gccDir
    }
    else {
        $newUserPath = "$gccDir;$userPath"
    }

    [Environment]::SetEnvironmentVariable(
        "Path",
        $newUserPath,
        "User"
    )

    Write-Host "Added $gccDir to the user PATH."
}

Refresh-Path

# --------------------------------------------------
# Final host checks
# --------------------------------------------------

Write-Host ""
Write-Host "============================================"
Write-Host "Host toolchain"
Write-Host "============================================"

if (Get-Command git -ErrorAction SilentlyContinue) {
    git --version
}
else {
    Write-Warning "Git is not available."
}

if (Get-Command go -ErrorAction SilentlyContinue) {
    go version
}
else {
    Write-Warning "Go is not available."
}

if (Get-Command node -ErrorAction SilentlyContinue) {
    node --version
}
else {
    Write-Warning "Node is not available."
}

if (Get-Command npm -ErrorAction SilentlyContinue) {
    npm --version
}
else {
    Write-Warning "npm is not available."
}

if (Get-Command gcc -ErrorAction SilentlyContinue) {
    gcc --version | Select-Object -First 1
}
else {
    Write-Warning "gcc is not visible on PATH."
}

Write-Host ""
Write-Host "Host bootstrap complete."
Write-Host ""
Write-Host "IMPORTANT:"
Write-Host "Restart the Codex Windows app now so its sandbox"
Write-Host "inherits the updated Windows PATH."