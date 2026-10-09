# Installs mindful-yt from its GitHub releases on Windows. In PowerShell:
#
#   irm https://raw.githubusercontent.com/abhiram-ar/mindful-yt/main/install.ps1 | iex
#
# It installs only mindful-yt.exe and adds its folder to your user PATH. mindful-yt
# itself offers to install yt-dlp, Node.js and ffmpeg the first time it runs.
# Run it again to update.
#
# Environment:
#   MINDFUL_YT_VERSION      release to install, e.g. v0.1.0 (default: the latest)
#   MINDFUL_YT_INSTALL_DIR  where to put mindful-yt.exe (default: %LOCALAPPDATA%\Programs\mindful-yt)
#   MINDFUL_YT_NO_PATH      set to 1 to leave PATH alone
#   MINDFUL_YT_BASE_URL     where to download from instead of GitHub (for testing)

# One script block, so that under "irm | iex" an error stops the install
# without closing the user's PowerShell window.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is much slower with its progress bar
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'abhiram-ar/mindful-yt'
    # The machine's CPU, as the registry records it: right even in a 32-bit or
    # emulated PowerShell. Not [RuntimeInformation]::OSArchitecture: in every
    # interactive session PSReadLine brings its own RuntimeInformation type,
    # which has no OSArchitecture, and PowerShell picks that one.
    $system = 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager\Environment'
    $cpu = @(
        (Get-ItemProperty -LiteralPath $system -Name PROCESSOR_ARCHITECTURE -ErrorAction SilentlyContinue).PROCESSOR_ARCHITECTURE,
        $env:PROCESSOR_ARCHITEW6432,
        $env:PROCESSOR_ARCHITECTURE
    ) | Where-Object { $_ } | Select-Object -First 1
    $arch = switch ($cpu) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "mindful-yt install: there's no mindful-yt build for this CPU ($cpu); there are builds for 64-bit Intel/AMD and ARM" }
    }

    $version = if ($env:MINDFUL_YT_VERSION) { $env:MINDFUL_YT_VERSION } else { 'latest' }
    $base = if ($version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$version" }
    if ($env:MINDFUL_YT_BASE_URL) { $base = $env:MINDFUL_YT_BASE_URL }
    $dir = if ($env:MINDFUL_YT_INSTALL_DIR) { $env:MINDFUL_YT_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\mindful-yt' }
    $asset = "mindful-yt_windows_$arch.zip"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("mindful-yt-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $asset ($version)..."
        $zip = Join-Path $tmp $asset
        $sums = Join-Path $tmp 'checksums.txt'
        foreach ($file in @(@("$base/$asset", $zip), @("$base/checksums.txt", $sums))) {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri $file[0] -OutFile $file[1]
            } catch {
                throw "mindful-yt install: couldn't download $($file[0]) ($($_.Exception.Message))"
            }
        }

        $want = Get-Content -LiteralPath $sums | ForEach-Object {
            $hash, $name = -split $_
            if ($name -eq $asset) { $hash.ToLower() }
        } | Select-Object -First 1
        if (-not $want) { throw "mindful-yt install: $asset isn't listed in checksums.txt" }
        $got = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLower()
        if ($got -ne $want) { throw "mindful-yt install: $asset failed its checksum, so it wasn't installed" }

        Expand-Archive -LiteralPath $zip -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $exe = Join-Path $dir 'mindful-yt.exe'
        if (Test-Path -LiteralPath $exe) {
            # A running mindful-yt.exe can't be replaced, but it can be renamed.
            $old = "$exe.old"
            Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
            Move-Item -LiteralPath $exe -Destination $old -Force
            Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
        }
        Move-Item -LiteralPath (Join-Path $tmp 'mindful-yt.exe') -Destination $exe
        $installed = & $exe --version
        Write-Host "Installed $installed to $exe"

        if ($env:MINDFUL_YT_NO_PATH -ne '1') {
            # Edit the raw registry value, so entries like %USERPROFILE%\... stay unexpanded.
            $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
            try {
                $path = [string]$key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
                $entries = $path -split ';' | ForEach-Object { $_.TrimEnd('\') }
                if ($entries -notcontains $dir.TrimEnd('\')) {
                    $key.SetValue('Path', ($path.TrimEnd(';') + ';' + $dir).TrimStart(';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
                    # Setting any user variable through .NET tells Windows the environment changed.
                    [Environment]::SetEnvironmentVariable('MINDFUL_YT_INSTALL', '1', 'User')
                    [Environment]::SetEnvironmentVariable('MINDFUL_YT_INSTALL', $null, 'User')
                    $env:Path = "$env:Path;$dir"
                    Write-Host "Added $dir to your PATH. New terminals will find mindful-yt."
                }
            } finally {
                $key.Close()
            }
        }
        Write-Host "Run mindful-yt. It offers to install yt-dlp, Node.js and ffmpeg if they're missing."
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}
