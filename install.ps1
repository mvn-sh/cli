$ErrorActionPreference = "Stop"

$Repo = "mvn-sh/cli"
$InstallDir = if ($env:MVNSH_INSTALL_DIR) { $env:MVNSH_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$Version = if ($env:MVNSH_VERSION) { $env:MVNSH_VERSION } else { "latest" }

try {
    $Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    $Arch = switch ($Architecture) {
        "x64" { "amd64" }
        "arm64" { "arm64" }
        default { throw "Unsupported architecture: $Architecture" }
    }

    if ($Version -eq "latest") {
        $ReleaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        $Version = $ReleaseInfo.tag_name
    }
    if (-not $Version.StartsWith("v")) { $Version = "v$Version" }

    $Release = $Version.TrimStart("v")
    $Archive = "mvnsh_${Release}_windows_${Arch}.zip"
    $BaseUrl = "https://github.com/$Repo/releases/download/$Version"
    $TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("mvnsh-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $TempDir | Out-Null

    Write-Host "Downloading mvnsh $Version for windows/$Arch..."
    $ArchivePath = Join-Path $TempDir $Archive
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"
    Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath

    $ChecksumLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "\s+$([regex]::Escape($Archive))$" } | Select-Object -First 1
    if (-not $ChecksumLine) { throw "Release checksum not found" }
    $Expected = ($ChecksumLine -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 $ArchivePath).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) { throw "Checksum verification failed" }

    Expand-Archive -Path $ArchivePath -DestinationPath $TempDir -Force
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item (Join-Path $TempDir "mvnsh.exe") (Join-Path $InstallDir "mvnsh.exe") -Force

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathParts = @($UserPath -split ";" | Where-Object { $_ })
    if ($InstallDir -notin $PathParts) {
        $NewPath = (($PathParts + $InstallDir) -join ";")
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use it."
    }
    Write-Host "Installed mvnsh to $InstallDir\mvnsh.exe"
}
catch {
    Write-Error "mvnsh installer: $($_.Exception.Message)"
    exit 1
}
finally {
    if ($TempDir -and (Test-Path $TempDir)) { Remove-Item -Recurse -Force $TempDir }
}
