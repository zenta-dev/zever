# install.ps1 - install the zever CLI and zever-lsp on Windows from GitHub releases.
#
# Run with:
#   powershell -ExecutionPolicy Bypass -File install.ps1 -Version v0.6.1
#
# If the file was downloaded from the web, unblock it first:
#   Unblock-File .\install.ps1
#
# Notes (repo ethos, verified facts):
# - Binaries come from GitHub releases (or a source build). Never
#   `go install <module>@version`: the repo commits `replace` directives,
#   so `go install @version` fails.
# - `zever -V` prints `zever vX.Y.Z`. zever-lsp has no CLI version flag
#   (stdio-only), so the LSP is matched blindly to the CLI version.
# - Never hangs: non-interactive shells get an error telling them to re-run
#   with -Yes instead of blocking on Read-Host.
# - Errors go to stderr. No sudo/elevation. No package managers.
# - Re-runs are idempotent.
[CmdletBinding()]
param(
  [string]$Version = "v0.6.1",
  [string]$Prefix = (Join-Path $HOME ".local"),
  [switch]$Yes,
  [switch]$Force,
  [Alias("WhatIf")][switch]$DryRun,
  [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ZeverRepo = "zenta-dev/zever"
$GoMinMajor = 1
$GoMinMinor = 27
$GoDlUrl = "https://go.dev/dl/"

function Write-Err([string]$Msg) {
  [Console]::Error.WriteLine("install.ps1: error: $Msg")
}
function Write-Inf([string]$Msg) {
  [Console]::Error.WriteLine("install.ps1: $Msg")
}
function Write-WarnMsg([string]$Msg) {
  [Console]::Error.WriteLine("install.ps1: warning: $Msg")
}
function Show-Usage {
  @"
Usage: install.ps1 [-Version vX.Y.Z] [-Prefix DIR] [-Yes] [-Force] [-DryRun] [-Help]

Install the zever CLI and zever-lsp from GitHub releases.

  -Version  version to install (default: v0.6.1)
  -Prefix   install prefix, binaries go to DIR\bin (default: `$HOME\.local)
  -Yes      accept upgrade/downgrade prompts without asking
  -Force    re-install even when the target version is already installed
            (implies -Yes and overwrites existing binaries)
  -DryRun   print every action without executing it (also via -WhatIf)
  -Help     print this help and exit

Examples:
  powershell -ExecutionPolicy Bypass -File install.ps1 -Version v0.6.1 -Yes
  .\install.ps1 -Prefix "$HOME\.local" -DryRun
"@
}

function Rerun-Command {
  return "powershell -ExecutionPolicy Bypass -File install.ps1 -Version $Version -Prefix $Prefix -Yes"
}

# Compare-SemVer A B -> -1, 0, or 1. Hand-rolled numeric field comparison.
function Compare-SemVer([string]$A, [string]$B) {
  $a = $A.TrimStart('v'); $b = $B.TrimStart('v')
  $aPre = ""; $bPre = ""
  if ($a -match '^(.*?)[-+](.*)$') { $a = $Matches[1]; $aPre = $Matches[2] }
  if ($b -match '^(.*?)[-+](.*)$') { $b = $Matches[1]; $bPre = $Matches[2] }
  $aF = $a.Split('.'); $bF = $b.Split('.')
  for ($i = 0; $i -lt 3; $i++) {
    $x = 0; $y = 0
    if ($i -lt $aF.Count) { [int]$x = $aF[$i] -replace '\D.*$', '' }
    if ($i -lt $bF.Count) { [int]$y = $bF[$i] -replace '\D.*$', '' }
    if ($x -gt $y) { return 1 }
    if ($x -lt $y) { return -1 }
  }
  if (($aPre -eq "") -and ($bPre -ne "")) { return 1 }
  if (($aPre -ne "") -and ($bPre -eq "")) { return -1 }
  return [string]::Compare($aPre, $bPre, [StringComparison]::Ordinal) | ForEach-Object {
    if ($_ -gt 0) { 1 } elseif ($_ -lt 0) { -1 } else { 0 }
  }
}

function Test-VersionShape([string]$V) {
  return $V -match '^v[0-9]+\.[0-9]+\.[0-9]+'
}

function Get-InstalledVersion([string]$ExePath) {
  try {
    $out = & $ExePath -V 2>$null
  } catch { return "" }
  $tok = ($out -split '\s+')[-1]
  if (Test-VersionShape $tok) { return $tok }
  return ""
}

# Read-Yes PROMPT: $true on explicit yes. Never hangs in CI: when input is
# redirected (or the session is not interactive) and -Yes was not passed,
# exit 1 with the exact re-run command instead of blocking on Read-Host.
function Read-Yes([string]$Prompt) {
  if ($Yes -or $Force) { return $true }
  $redirected = $false
  try { $redirected = [Console]::IsInputRedirected } catch { $redirected = $true }
  if ($redirected -or -not [Environment]::UserInteractive) {
    Write-Err "non-interactive shell: re-run with -Yes: $(Rerun-Command)"
    exit 1
  }
  $ans = Read-Host $Prompt
  return ($ans -match '^[Yy]')
}

if ($Help) { Show-Usage; exit 0 }
if ($Force) { $Yes = $true }

if (-not $Version.StartsWith("v")) { $Version = "v$Version" }
if (-not (Test-VersionShape $Version)) {
  Write-Err "invalid version: $Version (want vX.Y.Z)"
  exit 2
}
$BinDir = Join-Path $Prefix "bin"

# --- arch: only windows-amd64 ships for now ----------------------------------
$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($arch) {
  "X64" { $goarch = "amd64" }
  default {
    Write-Err "unsupported architecture: $arch (windows-amd64 only for now)"
    exit 1
  }
}
$ZeverAsset = "zever-windows-$goarch.exe"
$LspAsset = "zever-lsp-windows-$goarch.exe"
$ZeverExe = Join-Path $BinDir "zever.exe"
$LspExe = Join-Path $BinDir "zever-lsp.exe"
$BaseUrl = "https://github.com/$ZeverRepo/releases/download/$Version"

# --- prereq: go >= 1.27 --------------------------------------------------------
$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) {
  Write-Err "need go >= $GoMinMajor.$GoMinMinor (see $GoDlUrl)"
  exit 1
}
$goOut = (& go version) -join " "
if ($goOut -match 'go(\d+)\.(\d+)') {
  $maj = [int]$Matches[1]; $min = [int]$Matches[2]
  if (($maj -lt $GoMinMajor) -or (($maj -eq $GoMinMajor) -and ($min -lt $GoMinMinor))) {
    Write-Err "go $maj.$min is too old (need go >= $GoMinMajor.$GoMinMinor, see $GoDlUrl)"
    exit 1
  }
} else {
  Write-Err "could not parse go version from '$goOut' (need go >= $GoMinMajor.$GoMinMinor from $GoDlUrl)"
  exit 1
}

function Download-File([string]$Url, [string]$Dest) {
  if ($DryRun) {
    Write-Output "dry-run: download $Url -> $Dest"
    return
  }
  Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
}

# --- upgrade gate (BEFORE downloading anything) --------------------------------
$Existing = Get-Command zever -ErrorAction SilentlyContinue
if ($Existing) { $ExistingPath = $Existing.Source } else { $ExistingPath = "" }

if ($ExistingPath -ne "" -and ($ExistingPath -ne $ZeverExe)) {
  Write-WarnMsg "existing zever at $ExistingPath takes precedence over $ZeverExe per PATH order"
  Write-WarnMsg "this install writes $ZeverExe; fix your PATH if 'zever' still resolves elsewhere"
}

if ($ExistingPath -ne "") {
  $Have = Get-InstalledVersion $ExistingPath
  if ($Have -eq "") {
    Write-Inf "found existing zever at $ExistingPath but could not parse 'zever -V' output"
    if ($DryRun) {
      Write-Output "dry-run: would prompt to replace unparseable install at $ExistingPath"
    } elseif (-not (Read-Yes "Replace existing zever at $ExistingPath with $Version? [y/N]")) {
      Write-Inf "aborted by user"
      exit 1
    }
  } else {
    $cmp = Compare-SemVer $Have $Version
    if (($cmp -eq 0) -and (-not $Force)) {
      $binOk = (Test-Path $ZeverExe) -and ((Get-Item $ZeverExe).Length -gt 0)
      $lspOk = (Test-Path $LspExe) -and ((Get-Item $LspExe).Length -gt 0)
      if ($binOk -and $lspOk) {
        Write-Inf "zever $Version is already installed and up to date at $ZeverExe"
        exit 0
      }
      Write-WarnMsg "zever $Version reports installed but $ZeverExe or $LspExe is missing/empty; reinstalling"
    } elseif ($cmp -eq 0) {
      Write-Inf "-Force: reinstalling zever $Version over identical install"
    } elseif ($cmp -lt 0) {
      if ($DryRun) {
        Write-Output "dry-run: would prompt: Upgrade zever $Have -> $Version? [y/N] (assuming yes)"
      } elseif (-not (Read-Yes "Upgrade zever $Have -> $Version? [y/N]")) {
        Write-Inf "aborted by user"
        exit 1
      }
    } else {
      Write-WarnMsg "installed zever $Have is NEWER than target $Version"
      if ($DryRun) {
        Write-Output "dry-run: would prompt: Downgrade zever $Have -> $Version? [y/N] (assuming yes)"
      } elseif (-not (Read-Yes "Downgrade zever $Have -> $Version? [y/N]")) {
        Write-Inf "aborted by user"
        exit 1
      }
    }
  }
} else {
  Write-Inf "no existing zever found: fresh install of $Version"
}

# --- download + verify + install -----------------------------------------------
if ($DryRun) {
  Write-Output "dry-run: mkdir $BinDir"
  Write-Output "dry-run: download $BaseUrl/SHA256SUMS.txt -> <tmp>\SHA256SUMS.txt"
  Write-Output "dry-run: download $BaseUrl/$ZeverAsset -> $ZeverExe"
  Write-Output "dry-run: download $BaseUrl/$LspAsset -> $LspExe"
  Write-Output "dry-run: verify SHA256 of both binaries against SHA256SUMS.txt"
  Write-Output "dry-run: run gh attestation verify (only if gh exists)"
} else {
  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("zever-install-" + [IO.Path]::GetRandomFileName())
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  try {
    Write-Inf "downloading SHA256SUMS.txt for $Version"
    try {
      Download-File "$BaseUrl/SHA256SUMS.txt" (Join-Path $tmp "SHA256SUMS.txt")
    } catch {
      Write-Err "could not download $BaseUrl/SHA256SUMS.txt (release $Version may predate checksum files)"
      exit 1
    }
    $sums = Get-Content (Join-Path $tmp "SHA256SUMS.txt") -Raw
    foreach ($pair in @(@($ZeverAsset, $ZeverExe), @($LspAsset, $LspExe))) {
      $asset = $pair[0]; $dest = $pair[1]
      $tmpFile = Join-Path $tmp $asset
      Write-Inf "downloading $asset"
      try {
        Download-File "$BaseUrl/$asset" $tmpFile
      } catch {
        Write-Err "could not download $BaseUrl/$asset"
        exit 1
      }
      if (-not (Test-Path $tmpFile) -or (Get-Item $tmpFile).Length -eq 0) {
        Write-Err "downloaded $asset is empty"
        exit 1
      }
      $want = $null
      foreach ($line in ($sums -split "`r?`n")) {
        $t = $line.Trim() -split '\s+'
        $base = $t[-1].Split('/')[-1]
        if (($t.Count -ge 2) -and ($base -eq $asset)) { $want = $t[0]; break }
      }
      if (-not $want) {
        Write-Err "no checksum entry for $asset in SHA256SUMS.txt"
        exit 1
      }
      $got = (Get-FileHash -Algorithm SHA256 -Path $tmpFile).Hash.ToLower()
      if ($got -ne $want.ToLower()) {
        Write-Err "SHA256 mismatch for $asset (want $want, got $got)"
        exit 1
      }
      Move-Item -Force $tmpFile $dest
      if ((Get-Item $dest).Length -eq 0) {
        Write-Err "installed $dest is empty"
        exit 1
      }
    }
    Write-Inf "installed $ZeverExe and $LspExe"
    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if ($gh) {
      try { & gh attestation verify $ZeverExe --repo $ZeverRepo } catch {
        Write-WarnMsg "gh attestation verify failed for $ZeverExe"
      }
      try { & gh attestation verify $LspExe --repo $ZeverRepo } catch {
        Write-WarnMsg "gh attestation verify failed for $LspExe"
      }
    }
  } finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  }
}

# --- PATH: user-level, idempotent + current session ----------------------------
$needPath = $false
if ($DryRun) {
  Write-Output "dry-run: add $BinDir to User PATH idempotently + current session"
} else {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (-not $userPath) { $userPath = "" }
  $parts = $userPath -split ';' | Where-Object { $_ -ne '' }
  $found = $false
  foreach ($p in $parts) {
    if ($p.TrimEnd('\') -ieq $BinDir.TrimEnd('\')) { $found = $true; break }
  }
  if ($found) {
    Write-Inf "$BinDir is already on User PATH"
  } else {
    $newPath = if ($userPath -eq "") { $BinDir } else { "$userPath;$BinDir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Inf "added $BinDir to User PATH"
  }
  if (($env:Path -split ';' | Where-Object { $_ -ne '' } | ForEach-Object { $_.TrimEnd('\') }) -notcontains $BinDir.TrimEnd('\')) {
    $env:Path = "$env:Path;$BinDir"
  }
}

# --- smoke test -----------------------------------------------------------------
if ($DryRun) {
  Write-Output "dry-run: & `"$ZeverExe`" --help (must exit 0), report PASS/FAIL"
} else {
  $smokeDir = Join-Path ([IO.Path]::GetTempPath()) ("zever-smoke-" + [IO.Path]::GetRandomFileName())
  try {
    & $ZeverExe --help >$null 2>&1
    if ($LASTEXITCODE -ne 0) {
      Write-Err "smoke test FAIL: 'zever --help' exited $LASTEXITCODE"
      exit 1
    }
    $haveV = Get-InstalledVersion $ZeverExe
    if (($haveV -ne "") -and ($haveV -ne $Version)) {
      Write-WarnMsg "'zever -V' reports $haveV, expected $Version"
    }
    Write-Inf "smoke test PASS"
  } finally {
    Remove-Item -Recurse -Force $smokeDir -ErrorAction SilentlyContinue
  }
}

Write-Inf "installed zever $Version to $ZeverExe and $LspExe"
Write-Output "export PATH: $BinDir (already added to User PATH for future sessions)"
