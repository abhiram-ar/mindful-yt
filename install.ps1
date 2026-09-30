# Installs ytget from its GitHub releases on Windows. In PowerShell:
#
#   irm https://raw.githubusercontent.com/abhiram-ar/youtube-downloader-via-dns-over-http/main/install.ps1 | iex
#
# It installs only ytget.exe and adds its folder to your user PATH. ytget
# itself offers to install yt-dlp, Deno and ffmpeg the first time it runs.
# Run it again to update.
#
# Environment:
#   YTGET_VERSION      release to install, e.g. v0.1.0 (default: the latest)
#   YTGET_INSTALL_DIR  where to put ytget.exe (default: %LOCALAPPDATA%\Programs\ytget)
#   YTGET_NO_PATH      set to 1 to leave PATH alone
#   YTGET_BASE_URL     where to download from instead of GitHub (for testing)

# One script block, so that under "irm | iex" an error stops the install
# without closing the user's PowerShell window.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is much slower with its progress bar
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'abhiram-ar/youtube-downloader-via-dns-over-http'
    try {
        $cpu = [string][Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    } catch {
        $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    $arch = switch ($cpu) {
        { $_ -in 'X64', 'AMD64' } { 'amd64' }
        { $_ -in 'Arm64', 'ARM64' } { 'arm64' }
        default { throw "ytget install: unsupported CPU $cpu" }
    }

    $version = if ($env:YTGET_VERSION) { $env:YTGET_VERSION } else { 'latest' }
    $base = if ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
    if ($env:YTGET_BASE_URL) { $base = $env:YTGET_BASE_URL }
    $dir = if ($env:YTGET_INSTALL_DIR) { $env:YTGET_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ytget' }
    $asset = "ytget_windows_$arch.zip"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("ytget-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $asset ($version)..."
        $zip = Join-Path $tmp $asset
        $sums = Join-Path $tmp 'checksums.txt'
        foreach ($file in @(@("$base/$asset", $zip), @("$base/checksums.txt", $sums))) {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri $file[0] -OutFile $file[1]
            } catch {
                throw "ytget install: couldn't download $($file[0]) ($($_.Exception.Message))"
            }
        }

        $want = Get-Content -LiteralPath $sums | ForEach-Object {
            $hash, $name = -split $_
            if ($name -eq $asset) { $hash.ToLower() }
        } | Select-Object -First 1
        if (-not $want) { throw "ytget install: $asset isn't listed in checksums.txt" }
        $got = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLower()
        if ($got -ne $want) { throw "ytget install: $asset failed its checksum, so it wasn't installed" }

        Expand-Archive -LiteralPath $zip -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $exe = Join-Path $dir 'ytget.exe'
        if (Test-Path -LiteralPath $exe) {
            # A running ytget.exe can't be replaced, but it can be renamed.
            $old = "$exe.old"
            Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
            Move-Item -LiteralPath $exe -Destination $old -Force
            Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
        }
        Move-Item -LiteralPath (Join-Path $tmp 'ytget.exe') -Destination $exe
        $installed = & $exe --version
        Write-Host "Installed $installed to $exe"

        if ($env:YTGET_NO_PATH -ne '1') {
            # Edit the raw registry value, so entries like %USERPROFILE%\... stay unexpanded.
            $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
            try {
                $path = [string]$key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
                $entries = $path -split ';' | ForEach-Object { $_.TrimEnd('\') }
                if ($entries -notcontains $dir.TrimEnd('\')) {
                    $key.SetValue('Path', ($path.TrimEnd(';') + ';' + $dir).TrimStart(';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
                    # Setting any user variable through .NET tells Windows the environment changed.
                    [Environment]::SetEnvironmentVariable('YTGET_INSTALL', '1', 'User')
                    [Environment]::SetEnvironmentVariable('YTGET_INSTALL', $null, 'User')
                    $env:Path = "$env:Path;$dir"
                    Write-Host "Added $dir to your PATH. New terminals will find ytget."
                }
            } finally {
                $key.Close()
            }
        }
        Write-Host "Run ytget. It offers to install yt-dlp, Deno and ffmpeg if they're missing."
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}
