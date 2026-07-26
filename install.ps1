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

$version = $env:HELIKOPTER_VERSION
if (-not $version) {
    # Follow the /releases/latest redirect rather than calling the API, which
    # is rate limited for unauthenticated callers.
    $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" `
        -MaximumRedirection 0 -ErrorAction SilentlyContinue
    $loc = $resp.Headers.Location
    if (-not $loc) { Die 'could not determine the latest version' }
    $version = ($loc -split '/')[-1]
}

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

    Copy-Item (Join-Path $tmp "$Bin.exe") (Join-Path $dir "$Bin.exe") -Force
    Info "installed to $dir\$Bin.exe"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$dir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
        Info "added $dir to your PATH (restart your shell to pick it up)"
    }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host ''
Info 'done. take off with:'
Write-Host ''
Info '  helikopter'
Write-Host ''
