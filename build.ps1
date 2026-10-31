# Builds a portable TroopersTransfer.exe into .\dist
# Requirements: Go (see go.mod) and a MinGW-w64 gcc on PATH (Fyne needs cgo).
#   winget install GoLang.Go
#   winget install BrechtSanders.WinLibs.POSIX.UCRT
param(
    [string]$Version = "1.0.0",
    [string]$Arch = "amd64"
)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not (Get-Command go-winres -ErrorAction SilentlyContinue)) {
    go install github.com/tc-hib/go-winres@v0.3.3
    $env:PATH += ";$(go env GOPATH)\bin"
}
$env:CGO_ENABLED = "1"
$env:GOARCH = $Arch
New-Item -ItemType Directory -Force dist | Out-Null
$ldflags = "-s -w -H windowsgui -X github.com/troopers/transfer-service/ui.Version=$Version"
go build -trimpath -ldflags $ldflags -o dist\TroopersTransfer.exe .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

# Embed icon, version info and manifest after linking. (Linking a .syso
# instead clashes with the default manifest MinGW's gcc adds.)
go-winres patch --no-backup --in winres\winres.json --product-version $Version --file-version $Version dist\TroopersTransfer.exe
if ($LASTEXITCODE -ne 0) { throw "go-winres failed" }
Get-Item dist\TroopersTransfer.exe | Select-Object Name, @{n="MB";e={[math]::Round($_.Length/1MB,1)}}
