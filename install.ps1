# Burrow installer for Windows (until the winget package is available).
#
#   irm https://raw.githubusercontent.com/sandeeda/burrow/main/install.ps1 | iex
#
# Downloads the latest release for this PC from GitHub, checks its SHA-256
# against the published checksums, installs burrow.exe to
# %LOCALAPPDATA%\Programs\Burrow and adds that folder to your PATH.
# Set $env:BURROW_VERSION = 'v0.1.0' first to pick a version, or
# $env:BURROW_DOWNLOAD_URL to download from a mirror.

$ErrorActionPreference = 'Stop'
$repo = 'sandeeda/burrow'

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$version = $env:BURROW_VERSION
if (-not $version) {
    $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
$plain = $version.TrimStart('v')
$archive = "burrow_${plain}_windows_${arch}.zip"
$base = if ($env:BURROW_DOWNLOAD_URL) { $env:BURROW_DOWNLOAD_URL } else { "https://github.com/$repo/releases/download/$version" }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("burrow-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading Burrow $version for windows/$arch..."
    Invoke-WebRequest "$base/$archive" -OutFile "$tmp\$archive" -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing

    Write-Host 'Verifying checksum...'
    $line = Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match " $([regex]::Escape($archive))$" }
    if (-not $line) { throw "No checksum published for $archive" }
    $want = ($line -split ' ')[0]
    $got = (Get-FileHash "$tmp\$archive" -Algorithm SHA256).Hash.ToLower()
    if ($want -ne $got) { throw 'Checksum mismatch - the download may be corrupted or tampered with.' }

    $dir = Join-Path $env:LOCALAPPDATA 'Programs\Burrow'
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Expand-Archive "$tmp\$archive" -DestinationPath $tmp -Force
    Copy-Item "$tmp\burrow.exe" "$dir\burrow.exe" -Force

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not ($userPath -split ';' | Where-Object { $_ -eq $dir })) {
        [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ";$dir"), 'User')
        Write-Host "Added $dir to your PATH (open a new terminal to use it)."
    }
    Write-Host "Installed burrow $version to $dir\burrow.exe"
    Write-Host ''
    Write-Host 'Next: open a new terminal and run   burrow "$HOME\Documents"'
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
