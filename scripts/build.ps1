<#
  Build script for Crush (Windows PowerShell)

  - Builds Windows binaries with arch suffix into ./build
  - By default builds for the host arch; use -All to build amd64 and arm64
  - Optional install (-Install) to ~/.local/bin/crush.exe (no suffix)

  Examples:
    pwsh scripts/build.ps1
    pwsh scripts/build.ps1 -All
    pwsh scripts/build.ps1 -Install
    pwsh scripts/build.ps1 -Arch arm64 -Install
#>
[CmdletBinding()] param(
  [switch] $All,
  [switch] $Install,
  [ValidateSet('amd64','arm64')]
  [string] $Arch
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-HostArch() {
  switch -Regex ($env:PROCESSOR_ARCHITECTURE) {
    'ARM64' { 'arm64'; break }
    'AMD64' { 'amd64'; break }
    default { 'amd64' } # reasonable default on Windows
  }
}

function Save-And-SetEnv($name, $value) {
  $item = Get-Item -Path "Env:$name" -ErrorAction SilentlyContinue
  $prev = if ($null -ne $item) { $item.Value } else { $null }
  Set-Item -Path "Env:$name" -Value $value
  return $prev
}

function Restore-Env($name, $prev) {
  if ($null -eq $prev) {
    Remove-Item -Path "Env:$name" -ErrorAction SilentlyContinue
  } else {
    Set-Item -Path "Env:$name" -Value $prev
  }
}

function Build-One($arch) {
  $root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
  Set-Location $root
  $buildDir = Join-Path $root 'build'
  if (-not (Test-Path $buildDir)) { New-Item -ItemType Directory -Path $buildDir | Out-Null }

  $out = Join-Path $buildDir ("crush_win_{0}.exe" -f $arch)
  Write-Host "Building $out (GOOS=windows GOARCH=$arch)"

  $prevGOOS = Save-And-SetEnv 'GOOS' 'windows'
  $prevGOARCH = Save-And-SetEnv 'GOARCH' $arch
  $prevCGO = Save-And-SetEnv 'CGO_ENABLED' '0'
  try {
    # Ensure previous binary does not block overwrite on Windows.
    if (Test-Path $out) {
      try {
        Remove-Item -Force -ErrorAction Stop $out
      } catch {
        Write-Warning "Failed to remove existing '$out'. It may be in use."
      }
    }
    & go build -o $out .
  }
  finally {
    Restore-Env 'GOOS' $prevGOOS
    Restore-Env 'GOARCH' $prevGOARCH
    Restore-Env 'CGO_ENABLED' $prevCGO
  }
}

function Install-Host($arch) {
  $root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
  $buildDir = Join-Path $root 'build'
  $src = Join-Path $buildDir ("crush_win_{0}.exe" -f $arch)
  $binDir = Join-Path $HOME '.local/bin'
  if (-not (Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir -Force | Out-Null }
  $dest = Join-Path $binDir 'crush.exe'
  Write-Host "Installing $src -> $dest"
  Copy-Item -Force $src $dest
}

function Main() {
  if ($All) {
    Build-One 'amd64'
    Build-One 'arm64'
    if ($Install) {
      $hostArch = if ($Arch) { $Arch } else { Get-HostArch }
      Install-Host $hostArch
    }
    return
  }

  $archEff = if ($Arch) { $Arch } else { Get-HostArch }
  Build-One $archEff
  if ($Install) { Install-Host $archEff }
}

Main
