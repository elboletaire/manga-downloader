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

  # there's no windows/arm64 build, but ARM machines run amd64 binaries emulated
  $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'x86') { '386' } else { 'amd64' }

  Write-Host "Looking for the latest release..."
  $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
  $zip = $release.assets | Where-Object { $_.name -like "*-windows-$arch.zip" } | Select-Object -First 1
  $md5 = $release.assets | Where-Object { $_.name -eq "$($zip.name).md5" } | Select-Object -First 1
  if (-not $zip) { throw "No windows-$arch build found in release $($release.tag_name)." }

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
