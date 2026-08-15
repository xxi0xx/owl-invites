param(
    [Parameter(
        Mandatory = $true,
        Position = 0
    )]
    [string]$Command,

    [Parameter(
        Position = 1,
        ValueFromRemainingArguments = $true
    )]
    [string[]]$CommandArgs
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "codex-env.ps1")

Set-Location $env:OWL_CODEX_REPO_ROOT

if (-not (Get-Command $Command -ErrorAction SilentlyContinue)) {
    throw "Command '$Command' is not available."
}

& $Command @CommandArgs

$code = $LASTEXITCODE

if ($null -eq $code) {
    $code = 0
}

exit $code