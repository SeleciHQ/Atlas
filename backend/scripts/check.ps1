# Backend verification script. Requires Go 1.27.4+ on PATH.
# Paths are derived from this script's location; no machine-specific paths.
$ErrorActionPreference = "Stop"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go 1.27.4 or newer is required on PATH (see backend/README.md)."
}
if (-not (Get-Command gofmt -ErrorAction SilentlyContinue)) {
    throw "gofmt is required on PATH (ships with Go)."
}

Push-Location (Join-Path $PSScriptRoot "..")
try {
    $unformatted = & gofmt -l cmd internal
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    if ($unformatted) {
        Write-Error ("Unformatted Go files: " + ($unformatted -join ", "))
        exit 1
    }

    & go vet ./cmd/... ./internal/...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & go build ./cmd/... ./internal/...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & go test ./cmd/... ./internal/... -count=1
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
