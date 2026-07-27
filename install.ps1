# helikopter installer for Windows
#
#   irm https://raw.githubusercontent.com/hammadsaedi/helikopter/main/install.ps1 | iex
#
# Environment:
#   HELIKOPTER_VERSION   tag to install (default: latest release)
#   HELIKOPTER_BIN_DIR   install directory (default: %LOCALAPPDATA%\helikopter\bin)

$ErrorActionPreference = 'Stop'

$Repo = 'hammadsaedi/helikopter'
$Bin  = 'helikopter'

function Info($m) { Write-Host "  $m" }
function Die($m)  { Write-Error $m; exit 1 }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    'x86'   { '386' }
    default { Die "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}
$platform = "windows_$arch"

function Get-LatestVersion {
    # The release API returns the tag directly and behaves the same on Windows
    # PowerShell and PowerShell 7.
    try {
        $api = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" `
            -Headers @{ 'User-Agent' = 'helikopter-installer' }
        if ($api.tag_name) { return $api.tag_name }
    } catch {
        # Unauthenticated API calls are rate limited per address, so fall
        # through to the redirect, which costs no quota.
    }

    # /releases/latest redirects to the tag. PowerShell 7 raises the 302 as a
    # terminating error instead of returning the response, so the Location has
    # to be read off the exception; Windows PowerShell returns it normally.
    $loc = $null
    try {
        $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" `
            -MaximumRedirection 0 -ErrorAction Stop
        $loc = $resp.Headers.Location
    } catch {
        $r = $_.Exception.Response
        if ($r) {
            try { $loc = $r.Headers.Location } catch { }
            if (-not $loc) { try { $loc = $r.Headers['Location'] } catch { } }
        }
    }
    # A string on Windows PowerShell, a string array or Uri on PowerShell 7.
    if ($loc -is [array]) { $loc = $loc[0] }
    if ($loc) { return ($loc.ToString().TrimEnd('/') -split '/')[-1] }
    return $null
}

$version = $env:HELIKOPTER_VERSION
if (-not $version) { $version = Get-LatestVersion }
if (-not $version) { Die 'could not determine the latest version' }

Write-Host ''
Info "installing $Bin $version for $platform"

$plain   = $version -replace '^v', ''
$tarball = "${Bin}_${plain}_${platform}.zip"
$base    = "https://github.com/$Repo/releases/download/$version"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("helikopter-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    Info "downloading $tarball"
    Invoke-WebRequest -Uri "$base/$tarball" -OutFile (Join-Path $tmp $tarball)

    try {
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
        $want = (Get-Content (Join-Path $tmp 'checksums.txt') |
                 Where-Object { $_ -match [regex]::Escape($tarball) + '$' }) -split '\s+' |
                 Select-Object -First 1
        if ($want) {
            $got = (Get-FileHash (Join-Path $tmp $tarball) -Algorithm SHA256).Hash.ToLower()
            if ($got -ne $want.ToLower()) { Die 'checksum verification failed' }
            Info 'checksum ok'
        }
    } catch {
        Info 'checksum skipped (checksums.txt not published)'
    }

    Expand-Archive -Path (Join-Path $tmp $tarball) -DestinationPath $tmp -Force

    $dir = $env:HELIKOPTER_BIN_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'helikopter\bin' }
    New-Item -ItemType Directory -Path $dir -Force | Out-Null

    $exe = Join-Path $dir "$Bin.exe"
    Copy-Item (Join-Path $tmp "$Bin.exe") $exe -Force

    # Anything fetched over the web carries a Zone.Identifier stream marking it
    # as internet-sourced, and Expand-Archive passes that on to what it
    # extracts. SmartScreen then warns about an unrecognised publisher every
    # time the binary runs. The download came from a release whose checksum was
    # just verified above, so clear the mark.
    #
    # This does not satisfy Smart App Control, which requires a signature no
    # matter where the file came from. See the README on code signing.
    Unblock-File -Path $exe -ErrorAction SilentlyContinue

    Info "installed to $exe"

    # Persist the directory for future sessions. Split on ';' and compare whole
    # entries rather than substrings: a -like '*dir*' test matches a longer path
    # that merely starts the same, and appending blindly to an empty or
    # trailing-';' PATH leaves empty segments behind.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @()
    if ($userPath) { $parts = @($userPath -split ';' | Where-Object { $_ -ne '' }) }
    if ($parts -notcontains $dir) {
        [Environment]::SetEnvironmentVariable('Path', (($parts + $dir) -join ';'), 'User')
        Info "added $dir to your PATH"
    }

    # And update this session. SetEnvironmentVariable only writes the stored
    # value; it does not touch the running process, so without this the very
    # next command fails with "'helikopter' is not recognized" even though the
    # install worked. This script is normally run through `irm | iex`, so the
    # session being fixed is the one the user is typing into.
    if (($env:Path -split ';') -notcontains $dir) {
        $env:Path = "$env:Path;$dir"
    }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host ''
Info 'done. take off with:'
Write-Host ''
Info '  helikopter'
Write-Host ''
