<#
  Build script for Crush (PowerShell)

  - Builds binaries into ./build as crush_${os}_${arch}[.exe]
  - By default builds for host OS/arch; use -AllTargets for all supported combos
  - Windows-only convenience remains with -All (builds amd64 + arm64 for Windows)
  - Optional install (-Install) copies host binary to ~/.local/bin/crush[.exe]
  - Show help with -h/--help/-?

  Examples:
    pwsh scripts/build.ps1                      # host OS/arch
    pwsh scripts/build.ps1 -AllTargets          # windows/linux/darwin x amd64/arm64
    pwsh scripts/build.ps1 -All                 # windows amd64 + arm64
    pwsh scripts/build.ps1 -OS linux -Arch arm64
    pwsh scripts/build.ps1 -OS windows,linux -Arch amd64
    pwsh scripts/build.ps1 -Install             # install host binary
#>
[CmdletBinding()] param(
  [Alias('h','help','?')]
  [switch] $ShowHelp,

  [switch] $All,
  [switch] $AllTargets,
  [switch] $Install,

  [ValidateSet('windows','linux','darwin')]
  [string[]] $OS,
  [ValidateSet('amd64','arm64')]
  [string[]] $Arch
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

function Get-HostOS() {
  if ($IsWindows) { return 'windows' }
  if ($IsLinux)   { return 'linux' }
  if ($IsMacOS)   { return 'darwin' }
  # Fallback: assume Windows naming
  return 'windows'
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

function Build-One($os, $arch) {
  $root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
  Set-Location $root
  $buildDir = Join-Path $root 'build'
  if (-not (Test-Path $buildDir)) { New-Item -ItemType Directory -Path $buildDir | Out-Null }

  $ext = if ($os -eq 'windows') { '.exe' } else { '' }
  $out = Join-Path $buildDir ("crush_{0}_{1}{2}" -f $os, $arch, $ext)
  Write-Host "Building $out (GOOS=$os GOARCH=$arch)"

  $prevGOOS = Save-And-SetEnv 'GOOS' $os
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

function Install-Host($os, $arch) {
  $root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
  $buildDir = Join-Path $root 'build'
  $ext = if ($os -eq 'windows') { '.exe' } else { '' }
  $src = Join-Path $buildDir ("crush_{0}_{1}{2}" -f $os, $arch, $ext)
  $binDir = Join-Path $HOME '.local/bin'
  if (-not (Test-Path $binDir)) { New-Item -ItemType Directory -Path $binDir -Force | Out-Null }
  $dest = Join-Path $binDir ("crush{0}" -f $ext)
  Write-Host "Installing $src -> $dest"
  Copy-Item -Force $src $dest
}

function Main() {
  if ($ShowHelp) {
    @'
Crush build script

Usage:
  pwsh scripts/build.ps1 [options]

Options:
  -h, --help, -?         Show this help.
  -All                   Build Windows targets (amd64, arm64).
  -AllTargets            Build all supported OS/arch targets.
  -OS <list>             One or more OS: windows, linux, darwin.
  -Arch <list>           One or more arch: amd64, arm64.
  -Install               Install host OS/arch binary to ~/.local/bin.

Notes:
  - Without -OS/-Arch/-All/-AllTargets, builds for host OS/arch.
  - Artifacts are written to ./build as crush_${os}_${arch}[.exe].
'@ | Write-Host
    return
  }

  if ($All) {
    Build-One 'windows' 'amd64'
    Build-One 'windows' 'arm64'
    if ($Install) {
      $hostOS = Get-HostOS
      $hostArch = if ($Arch) { $Arch[0] } else { Get-HostArch }
      Install-Host $hostOS $hostArch
    }
    return
  }

  if ($AllTargets) {
    $oses = @('windows','linux','darwin')
    $arches = @('amd64','arm64')
    foreach ($o in $oses) { foreach ($a in $arches) { Build-One $o $a } }
    if ($Install) {
      $hostOS = Get-HostOS
      $hostArch = Get-HostArch
      Install-Host $hostOS $hostArch
    }
    return
  }

  $osList = if ($OS -and $OS.Count -gt 0) { $OS } else { @(Get-HostOS) }
  $archList = if ($Arch -and $Arch.Count -gt 0) { $Arch } else { @(Get-HostArch) }
  foreach ($o in $osList) { foreach ($a in $archList) { Build-One $o $a } }
  if ($Install) {
    $hostOS = Get-HostOS
    $hostArch = Get-HostArch
    Install-Host $hostOS $hostArch
  }
}

Main
