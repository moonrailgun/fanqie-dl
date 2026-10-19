#requires -Version 5.1
$ErrorActionPreference = 'Stop'
if ($args.Count) { throw 'Usage: .\install.ps1' }
if ($env:OS -ne 'Windows_NT') { throw 'Use install.sh on systems other than Windows' }
$architecture = $env:PROCESSOR_ARCHITEW6432
if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
switch ($architecture) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { throw 'Unsupported architecture; expected AMD64 or ARM64' }
}
$installDir = $env:FANQIE_DL_INSTALL_DIR
if (-not $installDir) { $installDir = Join-Path $env:LOCALAPPDATA 'Programs/fanqie-dl' }
$installDir = [IO.Path]::GetFullPath($installDir)
$target = Join-Path $installDir 'fanqie-dl.exe'
if (Test-Path -LiteralPath $target -PathType Container) { throw "$target is a directory" }
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$release = Invoke-RestMethod -Uri 'https://api.github.com/repos/moonrailgun/fanqie-dl/releases/latest' -TimeoutSec 120
if ($release.tag_name -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
    throw 'Cannot determine latest release version; check whether a stable release has been published'
}
$version = $release.tag_name.Substring(1)
$archive = "fanqie-dl_${version}_windows_${arch}.zip"
$base = "https://github.com/moonrailgun/fanqie-dl/releases/download/v$version"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ('fanqie-dl-' + [guid]::NewGuid())
$staged = $null
try {
    $null = New-Item -ItemType Directory -Path $tmp
    Write-Host "Downloading fanqie-dl $version (windows/$arch)..."
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive) -TimeoutSec 120
    Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -TimeoutSec 120
    $checksumLines = @(Get-Content -LiteralPath (Join-Path $tmp 'checksums.txt') | Where-Object {
        $_ -match ('^[a-fA-F0-9]{64}\s+\*?' + [regex]::Escape($archive) + '$')
    })
    $actual = (Get-FileHash -LiteralPath (Join-Path $tmp $archive) -Algorithm SHA256).Hash
    if ($checksumLines.Count -ne 1 -or ($checksumLines[0] -split '\s+')[0] -ne $actual) {
        throw 'Release checksum verification failed'
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = $null
    try {
        $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $tmp $archive))
        $binary = $zip.GetEntry('fanqie-dl.exe')
        if (-not $binary -or $binary.Length -eq 0) { throw 'Release archive does not contain a binary' }
        $null = [IO.Directory]::CreateDirectory($installDir)
        $staged = Join-Path $installDir ('.fanqie-dl-' + [guid]::NewGuid() + '.tmp')
        [IO.Compression.ZipFileExtensions]::ExtractToFile($binary, $staged)
    } catch {
        throw "Failed to extract release archive: $_"
    } finally {
        if ($zip) { $zip.Dispose() }
    }
    # Stage beside the destination so failed upgrades keep the existing binary.
    if ([IO.File]::Exists($target)) {
        [IO.File]::Replace($staged, $target, [NullString]::Value)
    } else {
        [IO.File]::Move($staged, $target)
    }
    $staged = $null
    [string]$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (($userPath -split ';') -notcontains $installDir) {
        [Environment]::SetEnvironmentVariable('Path', (($userPath.TrimEnd(';') + ';' + $installDir).TrimStart(';')), 'User')
    }
    if (($env:PATH -split ';') -notcontains $installDir) { $env:PATH += ";$installDir" }
    Write-Host "Installed fanqie-dl $version to $target"
    Write-Host 'Run: fanqie-dl --version (reopen other terminals to refresh PATH)'
} finally {
    if ($staged -and (Test-Path -LiteralPath $staged)) { Remove-Item -LiteralPath $staged -Force }
    if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Recurse -Force }
}
