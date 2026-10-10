# Installs (or updates) the latest manga-downloader release for the current user.
#
#   irm https://raw.githubusercontent.com/elboletaire/manga-downloader/master/install.ps1 | iex
#
# The binary goes to %LOCALAPPDATA%\Programs\manga-downloader, which is added to
# the user Path. No administrator rights are needed; run it again to update.

& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue' # makes Invoke-WebRequest much faster on PS 5.1
  # Windows PowerShell 5.1 may default to TLS versions GitHub no longer accepts
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

  $repo = 'elboletaire/manga-downloader'
  $installDir = Join-Path $env:LOCALAPPDATA 'Programs\manga-downloader'

  # the native architecture, not the process's: a 32-bit PowerShell on 64-bit Windows
  # reports x86, and an x64 PowerShell emulated on ARM64 reports AMD64
  function Get-WindowsArchitecture {
    $arch = $null
    try {
      $arch = (Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment' -Name PROCESSOR_ARCHITECTURE -ErrorAction Stop).PROCESSOR_ARCHITECTURE
    } catch {}
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITEW6432 }
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
    $arch
  }

  # asset suffixes to try, in order: the first one present in the latest release wins
  function Get-AssetSuffixes($arch) {
    switch ($arch) {
      # no arm64 build in releases before it was added; ARM machines run amd64 emulated
      'ARM64' { 'arm64', 'amd64' }
      'AMD64' { 'amd64' }
      # older releases named the 32-bit build 386
      'x86'   { '32bit-legacy', '386' }
      default { throw "Unsupported Windows architecture: '$arch'." }
    }
  }

  $suffixes = @(Get-AssetSuffixes (Get-WindowsArchitecture))

  Write-Host "Looking for the latest release..."
  $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
  $zip = $null
  foreach ($suffix in $suffixes) {
    # the desktop app's zips share the suffix, so they're excluded by name
    $zip = $release.assets |
      Where-Object { $_.name -like "*-windows-$suffix.zip" -and $_.name -notlike 'manga-downloader-gui-*' } |
      Select-Object -First 1
    if ($zip) { break }
  }
  if (-not $zip) { throw "No windows build ($($suffixes -join ', ')) found in release $($release.tag_name)." }
  $md5 = $release.assets | Where-Object { $_.name -eq "$($zip.name).md5" } | Select-Object -First 1

  $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
  New-Item -ItemType Directory -Path $tmp | Out-Null
  try {
    Write-Host "Downloading $($zip.name)..."
    $zipPath = Join-Path $tmp $zip.name
    Invoke-WebRequest $zip.browser_download_url -OutFile $zipPath -UseBasicParsing

    if ($md5) {
      $expected = ((Invoke-RestMethod $md5.browser_download_url) -split '\s+')[0]
      $actual = (Get-FileHash $zipPath -Algorithm MD5).Hash
      if ($actual -ne $expected) { throw "Checksum mismatch for $($zip.name), aborting." }
    }

    Expand-Archive $zipPath -DestinationPath (Join-Path $tmp 'out')
    $exe = Get-ChildItem (Join-Path $tmp 'out') -Recurse -Filter 'manga-downloader.exe' | Select-Object -First 1
    if (-not $exe) { throw "manga-downloader.exe not found in $($zip.name)." }

    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    $target = Join-Path $installDir 'manga-downloader.exe'
    Copy-Item $exe.FullName $target -Force
    # drop the "downloaded from the internet" mark so SmartScreen doesn't block it
    Unblock-File $target
  } finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
  }

  # read the raw registry value: [Environment]::GetEnvironmentVariable expands
  # entries like %USERPROFILE%\bin, and writing that back would flatten them
  $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
  $userPath = [string]$key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
  $entries = @($userPath -split ';' | ForEach-Object { [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') })
  if ($entries -notcontains $installDir) {
    $kind = if ($userPath) { $key.GetValueKind('Path') } else { 'ExpandString' }
    $key.SetValue('Path', (($userPath.TrimEnd(';'), $installDir) -join ';').TrimStart(';'), $kind)
    # setting (and clearing) any variable through .NET broadcasts WM_SETTINGCHANGE,
    # so new terminals pick up the updated Path without logging out
    [Environment]::SetEnvironmentVariable('MANGA_DOWNLOADER_INSTALL', '1', 'User')
    [Environment]::SetEnvironmentVariable('MANGA_DOWNLOADER_INSTALL', $null, 'User')
    Write-Host "Added $installDir to your Path."
  }
  $key.Close()
  # make it usable right away in this window too
  if (($env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }) -notcontains $installDir) {
    $env:Path = "$env:Path;$installDir"
  }

  Write-Host ""
  Write-Host "manga-downloader $($release.tag_name) installed to $installDir" -ForegroundColor Green
  Write-Host "Open a new terminal and run: manga-downloader <url> <chapters>"
}
