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
# - All UI goes to stderr. Colour only on a VT-capable, non-redirected stderr,
#   never when NO_COLOR is set or -NoColor is passed.
[CmdletBinding()]
param(
  [string]$Version = "v0.6.1",
  [string]$Prefix = (Join-Path $HOME ".local"),
  [switch]$Yes,
  [switch]$Force,
  [Alias("WhatIf")][switch]$DryRun,
  [switch]$Quiet,
  [switch]$NoColor,
  [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ZeverRepo = "zenta-dev/zever"
$GoMinMajor = 1
$GoMinMinor = 27
$GoDlUrl = "https://go.dev/dl/"
$StepN = 0
$StepTotal = 5
$Stopwatch = [Diagnostics.Stopwatch]::StartNew()

# --- UI ------------------------------------------------------------------------
$useColor = $false
if (-not $NoColor -and -not $env:NO_COLOR -and $env:TERM -ne "dumb") {
  $errRedirected = $false
  try { $errRedirected = [Console]::IsErrorRedirected } catch { $errRedirected = $true }
  $vt = ($PSVersionTable.PSVersion.Major -ge 7) -or $env:WT_SESSION -or ($env:ConEmuANSI -eq "ON") -or $env:TERM
  if ((-not $errRedirected -and $vt) -or $env:FORCE_COLOR) { $useColor = $true }
}
if ($useColor) {
  $esc = [char]27
  $cReset = "$esc[0m"; $cBold = "$esc[1m"; $cDim = "$esc[2m"
  $cRed = "$esc[31m"; $cGreen = "$esc[32m"; $cYellow = "$esc[33m"; $cCyan = "$esc[36m"
} else {
  $cReset = ""; $cBold = ""; $cDim = ""; $cRed = ""; $cGreen = ""; $cYellow = ""; $cCyan = ""
}
$utf8 = $false
try { $utf8 = ($PSVersionTable.PSVersion.Major -ge 7) -or ([Console]::OutputEncoding.CodePage -eq 65001) } catch { $utf8 = $false }
if ($utf8) {
  $sOk = [string][char]0x2713; $sFail = [string][char]0x2717; $sStep = [string][char]0x2192
  $sDot = [string][char]0x2022; $sWould = [string][char]0x25CB; $sArrow = [string][char]0x2192
} else {
  $sOk = "ok"; $sFail = "x"; $sStep = ">"; $sDot = "-"; $sWould = "o"; $sArrow = "->"
}

function Say([string]$Msg) {
  if (-not $Quiet) { [Console]::Error.WriteLine($Msg) }
}
function Write-Inf([string]$Msg) { Say "  $cDim$sDot$cReset $Msg" }
function Write-Ok([string]$Msg) { Say "  $cGreen$sOk$cReset $Msg" }
function Write-Would([string]$Msg) { Say "  $cDim$sWould would $Msg$cReset" }
function Write-Kv([string]$Key, [string]$Val) { Say ("  {0}{1,-10}{2} {3}" -f $cDim, $Key, $cReset, $Val) }
function Write-Hint([string]$Msg) { [Console]::Error.WriteLine("  ${cDim}hint:$cReset $Msg") }
function Write-WarnMsg([string]$Msg) { [Console]::Error.WriteLine("$cYellow! warning:$cReset $Msg") }
function Write-Err([string]$Msg) { [Console]::Error.WriteLine("$cRed$sFail error:$cReset $Msg") }
function Write-Step([string]$Msg) {
  $script:StepN++
  Say ""
  Say "$cBold$cCyan$sStep [$($script:StepN)/$StepTotal]$cReset $cBold$Msg$cReset"
}
# Stop-Install MSG [HINT] [CODE]: report a failure with an optional next step.
function Stop-Install([string]$Msg, [string]$HintMsg = "", [int]$Code = 1) {
  Write-Err $Msg
  if ($HintMsg) { Write-Hint $HintMsg }
  exit $Code
}

function Format-Size([long]$Bytes) {
  if ($Bytes -ge 1MB) { return ("{0:N1} MB" -f ($Bytes / 1MB)) }
  if ($Bytes -ge 1KB) { return ("{0:N0} KB" -f ($Bytes / 1KB)) }
  return "$Bytes B"
}

function Show-Usage {
  @"
Install the zever CLI and zever-lsp from GitHub releases.

Usage: install.ps1 [-Version vX.Y.Z] [-Prefix DIR] [-Yes] [-Force] [-DryRun]
                   [-Quiet] [-NoColor] [-Help]

  -Version  version to install (default: v0.6.1)
  -Prefix   install prefix, binaries go to DIR\bin (default: `$HOME\.local)
  -Yes      accept upgrade/downgrade prompts without asking
  -Force    re-install even when the target version is already installed
            (implies -Yes and overwrites existing binaries)
  -DryRun   show every action without executing it (also via -WhatIf)
  -Quiet    only print warnings and errors
  -NoColor  disable colour output (also honours `$env:NO_COLOR)
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
    Stop-Install "non-interactive shell: confirmation required" "re-run with -Yes: $(Rerun-Command)"
  }
  $ans = Read-Host "$cBold${cCyan}?$cReset $Prompt"
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

Say ""
Say "${cBold}zever installer$cReset $cDim$Version$cReset"
if ($DryRun) { Say "${cYellow}dry run$cReset $cDim- nothing will be changed$cReset" }

# --- arch: only windows-amd64 ships for now ----------------------------------
$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($arch) {
  "X64" { $goarch = "amd64" }
  default {
    Stop-Install "unsupported architecture: $arch" "windows-amd64 only for now"
  }
}
$ZeverAsset = "zever-windows-$goarch.exe"
$LspAsset = "zever-lsp-windows-$goarch.exe"
$ZeverExe = Join-Path $BinDir "zever.exe"
$LspExe = Join-Path $BinDir "zever-lsp.exe"
$BaseUrl = "https://github.com/$ZeverRepo/releases/download/$Version"
$ReleaseHint = "check that release $Version exists: https://github.com/$ZeverRepo/releases"

# --- prereq: go >= 1.27 --------------------------------------------------------
$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) {
  Stop-Install "go not found (need go >= $GoMinMajor.$GoMinMinor)" "install Go from $GoDlUrl, then re-run"
}
$goOut = (& go version) -join " "
$goVer = ""
if ($goOut -match 'go(\d+)\.(\d+)(\.\d+)?') {
  $maj = [int]$Matches[1]; $min = [int]$Matches[2]
  $goVer = "$maj.$min" + $(if ($Matches[3]) { $Matches[3] } else { "" })
  if (($maj -lt $GoMinMajor) -or (($maj -eq $GoMinMajor) -and ($min -lt $GoMinMinor))) {
    Stop-Install "go $maj.$min is too old (need go >= $GoMinMajor.$GoMinMinor)" "upgrade Go from $GoDlUrl, then re-run"
  }
} else {
  Stop-Install "could not parse go version from '$goOut'" "need go >= $GoMinMajor.$GoMinMinor from $GoDlUrl"
}

function Download-File([string]$Url, [string]$Dest) {
  # Windows PowerShell 5.1 renders the progress bar ~10x slower than the
  # transfer itself, so only show it on PowerShell 7+ with an interactive host.
  $prev = $ProgressPreference
  if (($PSVersionTable.PSVersion.Major -lt 7) -or $Quiet -or [Console]::IsErrorRedirected) {
    $ProgressPreference = "SilentlyContinue"
  }
  try {
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
  } finally {
    $ProgressPreference = $prev
  }
}

# --- upgrade gate (BEFORE downloading anything) --------------------------------
$Existing = Get-Command zever -ErrorAction SilentlyContinue
if ($Existing) { $ExistingPath = $Existing.Source } else { $ExistingPath = "" }

$Have = ""
$Action = "fresh install"
$Gate = "fresh"
if ($ExistingPath -ne "") {
  $Have = Get-InstalledVersion $ExistingPath
  if ($Have -eq "") {
    $Action = "replace unrecognized install"; $Gate = "unparseable"
  } else {
    $cmp = Compare-SemVer $Have $Version
    if (($cmp -eq 0) -and (-not $Force)) {
      $binOk = (Test-Path $ZeverExe) -and ((Get-Item $ZeverExe).Length -gt 0)
      $lspOk = (Test-Path $LspExe) -and ((Get-Item $LspExe).Length -gt 0)
      if ($binOk -and $lspOk) { $Action = "already up to date"; $Gate = "uptodate" }
      else { $Action = "repair (binaries missing or empty)"; $Gate = "repair" }
    } elseif ($cmp -eq 0) {
      $Action = "reinstall (-Force)"; $Gate = "force"
    } elseif ($cmp -lt 0) {
      $Action = "upgrade $Have $sArrow $Version"; $Gate = "upgrade"
    } else {
      $Action = "downgrade $Have $sArrow $Version"; $Gate = "downgrade"
    }
  }
}

Say ""
Say "${cBold}Plan$cReset"
Write-Kv "version" $Version
Write-Kv "platform" "windows-$goarch"
Write-Kv "install to" $BinDir
Write-Kv "go" "$goVer $cDim(>= $GoMinMajor.$GoMinMinor required)$cReset"
if ($ExistingPath -ne "") {
  $haveSuffix = if ($Have -ne "") { " ($Have)" } else { "" }
  Write-Kv "existing" "$ExistingPath$haveSuffix"
}
Write-Kv "action" "$cBold$Action$cReset"

if ($ExistingPath -ne "" -and ($ExistingPath -ne $ZeverExe)) {
  Write-WarnMsg "existing zever at $ExistingPath takes precedence over $ZeverExe per PATH order"
  Write-Hint "this install writes $ZeverExe; fix your PATH if 'zever' still resolves elsewhere"
}

switch ($Gate) {
  "uptodate" {
    Say ""
    Write-Ok "zever $Version is already installed and up to date at $ZeverExe"
    Write-Hint "use -Force to reinstall"
    exit 0
  }
  "repair" {
    Write-WarnMsg "zever $Version reports installed but $ZeverExe or $LspExe is missing/empty; reinstalling"
  }
  "unparseable" {
    if ($DryRun) {
      Write-Would "prompt to replace unparseable install at $ExistingPath"
    } elseif (-not (Read-Yes "Replace existing zever at $ExistingPath with $Version? [y/N]")) {
      Stop-Install "aborted by user"
    }
  }
  "upgrade" {
    if ($DryRun) {
      Write-Would "prompt: Upgrade zever $Have $sArrow $Version? (assuming yes)"
    } elseif (-not (Read-Yes "Upgrade zever $Have $sArrow $Version? [y/N]")) {
      Stop-Install "aborted by user"
    }
  }
  "downgrade" {
    Write-WarnMsg "installed zever $Have is NEWER than target $Version"
    if ($DryRun) {
      Write-Would "prompt: Downgrade zever $Have $sArrow $Version? (assuming yes)"
    } elseif (-not (Read-Yes "Downgrade zever $Have $sArrow $Version? [y/N]")) {
      Stop-Install "aborted by user"
    }
  }
}

# --- download + verify + install -----------------------------------------------
$tmp = $null
try {
  if (-not $DryRun) {
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("zever-install-" + [IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  }

  Write-Step "Fetching checksums"
  $sums = ""
  if ($DryRun) {
    Write-Would "download $BaseUrl/SHA256SUMS.txt"
  } else {
    try {
      Download-File "$BaseUrl/SHA256SUMS.txt" (Join-Path $tmp "SHA256SUMS.txt")
    } catch {
      Stop-Install "could not download $BaseUrl/SHA256SUMS.txt" "$ReleaseHint (older releases may predate checksum files)"
    }
    $sums = Get-Content (Join-Path $tmp "SHA256SUMS.txt") -Raw
    Write-Ok "SHA256SUMS.txt for $Version"
  }

  foreach ($item in @(
      @{ Asset = $ZeverAsset; Dest = $ZeverExe; Label = "zever" },
      @{ Asset = $LspAsset; Dest = $LspExe; Label = "zever-lsp" })) {
    $asset = $item.Asset; $dest = $item.Dest
    Write-Step "Installing $($item.Label)"
    if ($DryRun) {
      Write-Would "download $BaseUrl/$asset"
      Write-Would "verify SHA256 against SHA256SUMS.txt"
      Write-Would "install to $dest"
      continue
    }
    $tmpFile = Join-Path $tmp $asset
    Write-Inf "downloading $asset"
    try {
      Download-File "$BaseUrl/$asset" $tmpFile
    } catch {
      Stop-Install "could not download $BaseUrl/$asset" $ReleaseHint
    }
    if (-not (Test-Path $tmpFile) -or (Get-Item $tmpFile).Length -eq 0) {
      Stop-Install "downloaded $asset is empty"
    }
    $size = Format-Size (Get-Item $tmpFile).Length
    $want = $null
    foreach ($line in ($sums -split "`r?`n")) {
      $t = $line.Trim() -split '\s+'
      $base = $t[-1].Split('/')[-1]
      if (($t.Count -ge 2) -and ($base -eq $asset)) { $want = $t[0]; break }
    }
    if (-not $want) {
      Stop-Install "no checksum entry for $asset in SHA256SUMS.txt"
    }
    $got = (Get-FileHash -Algorithm SHA256 -Path $tmpFile).Hash.ToLower()
    if ($got -ne $want.ToLower()) {
      Stop-Install "SHA256 mismatch for $asset (want $want, got $got)" "do NOT use this binary; retry, and report it if it persists"
    }
    Write-Ok "verified $asset $cDim($size, sha256 $($got.Substring(0, 12))...)$cReset"
    Move-Item -Force $tmpFile $dest
    if ((Get-Item $dest).Length -eq 0) {
      Stop-Install "installed $dest is empty"
    }
    Write-Ok "installed $dest"
  }

  if ($DryRun) {
    Write-Would "run gh attestation verify (only if gh exists)"
  } elseif (Get-Command gh -ErrorAction SilentlyContinue) {
    Write-Inf "verifying build provenance with gh"
    foreach ($bin in @($ZeverExe, $LspExe)) {
      $attested = $false
      try {
        & gh attestation verify $bin --repo $ZeverRepo *> $null
        $attested = ($LASTEXITCODE -eq 0)
      } catch { $attested = $false }
      if ($attested) { Write-Ok "attestation verified for $(Split-Path $bin -Leaf)" }
      else { Write-WarnMsg "gh attestation verify failed for $bin" }
    }
  }
} finally {
  if ($tmp) { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
}

# --- PATH: user-level, idempotent + current session ----------------------------
Write-Step "Configuring PATH"
$pathChanged = $false
if ($DryRun) {
  Write-Would "add $BinDir to User PATH idempotently + current session"
} else {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if (-not $userPath) { $userPath = "" }
  $parts = $userPath -split ';' | Where-Object { $_ -ne '' }
  $found = $false
  foreach ($p in $parts) {
    if ($p.TrimEnd('\') -ieq $BinDir.TrimEnd('\')) { $found = $true; break }
  }
  if ($found) {
    Write-Ok "$BinDir is already on User PATH"
  } else {
    $newPath = if ($userPath -eq "") { $BinDir } else { "$userPath;$BinDir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $pathChanged = $true
    Write-Ok "added $BinDir to User PATH"
  }
  if (($env:Path -split ';' | Where-Object { $_ -ne '' } | ForEach-Object { $_.TrimEnd('\') }) -notcontains $BinDir.TrimEnd('\')) {
    $env:Path = "$env:Path;$BinDir"
  }
}

# --- smoke test -----------------------------------------------------------------
Write-Step "Smoke test"
if ($DryRun) {
  Write-Would "run `"$ZeverExe`" --help (must exit 0) and -V, report PASS/FAIL"
} else {
  & $ZeverExe --help >$null 2>&1
  if ($LASTEXITCODE -ne 0) {
    Stop-Install "smoke test failed: 'zever --help' exited $LASTEXITCODE" "binaries are installed at $ZeverExe; run 'zever doctor' to diagnose"
  }
  Write-Ok "zever --help"
  $haveV = Get-InstalledVersion $ZeverExe
  if (($haveV -ne "") -and ($haveV -ne $Version)) {
    Write-WarnMsg "'zever -V' reports $haveV, expected $Version"
  } elseif ($haveV -ne "") {
    Write-Ok "zever -V $cDim($haveV)$cReset"
  }
}

# --- done ------------------------------------------------------------------------
Say ""
if ($DryRun) {
  Say "$cBold$cYellow$sWould Dry run complete$cReset $cDim- zever $Version was not installed$cReset"
  exit 0
}
$secs = [int]$Stopwatch.Elapsed.TotalSeconds
Say "$cBold$cGreen$sOk zever $Version installed$cReset ${cDim}in ${secs}s$cReset"
Say ""
Write-Kv "zever" $ZeverExe
Write-Kv "zever-lsp" $LspExe
Say ""
Say "${cBold}Next steps$cReset"
if ($pathChanged) {
  Say "  ${cDim}$sDot$cReset User PATH updated; this session is ready, ${cDim}open a new terminal for others$cReset"
}
Say "  ${cDim}1.$cReset ${cCyan}zever -V$cReset              ${cDim}check the install$cReset"
Say "  ${cDim}2.$cReset ${cCyan}zever new myapp$cReset       ${cDim}scaffold a project$cReset"
Say "  ${cDim}$sDot$cReset ${cCyan}zever upgrade$cReset / ${cCyan}zever uninstall$cReset  ${cDim}manage this install later$cReset"
