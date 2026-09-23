[CmdletBinding()]
param(
    [string]$DistDir = "dist-go",
    [string]$ChangelogFile = "CHANGELOG.md",
    [switch]$IncludeModel
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$rootDir = (Get-Item $scriptDir).Parent.FullName

Write-Host "===================================================" -ForegroundColor Cyan
Write-Host "       PopTrans Release Package Packer" -ForegroundColor Cyan
Write-Host "===================================================" -ForegroundColor Cyan
Write-Host ""

$distPath = Join-Path $rootDir $DistDir
$targetExe = Join-Path $distPath "PopTrans.exe"

# 1. Verify dist-go ready
if (-not (Test-Path $targetExe)) {
    Write-Host "[ERROR] PopTrans.exe not found in $distPath!" -ForegroundColor Red
    Write-Host "Please run scripts\build_all.bat to build the application first." -ForegroundColor Yellow
    exit 1
}

# 2. Extract latest version from CHANGELOG.md
$version = "v1.0.0"
$changelogPath = Join-Path $rootDir $ChangelogFile
if (Test-Path $changelogPath) {
    $match = (Get-Content -Path $changelogPath -Encoding UTF8 | Select-String -Pattern '^\#\#\s*\[([^\]]+)\]' | Select-Object -First 1)
    if ($match -and $match.Matches.Groups.Count -gt 1) {
        $version = $match.Matches.Groups[1].Value.Trim()
    }
} else {
    Write-Host "[WARNING] $ChangelogFile not found, fallback to version $version" -ForegroundColor Yellow
}

Write-Host "[INFO] Latest Version from Changelog: $version" -ForegroundColor Green

# 3. Target zip archive name and path
$zipName = "PopTrans-$version.zip"
$zipPath = Join-Path $rootDir $zipName

if (Test-Path $zipPath) {
    Write-Host "[INFO] Overwriting existing archive: $zipName" -ForegroundColor Yellow
    Remove-Item -Path $zipPath -Force
}

if ($IncludeModel) {
    Write-Host "[INFO] Packaging $DistDir (including local AI models) into $zipName..." -ForegroundColor Cyan
} else {
    Write-Host "[INFO] Packaging $DistDir (excluding large AI model *.gguf) into $zipName..." -ForegroundColor Cyan
}

# 4. Use Windows native .NET ZipFile to produce 100% compliant standard ZIP archives
Add-Type -AssemblyName System.IO.Compression.FileSystem

$tempStaging = Join-Path ([System.IO.Path]::GetTempPath()) ("poptrans_staging_" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempStaging -Force | Out-Null

try {
    Write-Host "[INFO] Staging files for packaging..." -ForegroundColor Cyan
    if ($IncludeModel) {
        robocopy $distPath $tempStaging /E /XF *.log settings.json capture_selection.py translate-wails.exe /XD __pycache__ > $null
    } else {
        robocopy $distPath $tempStaging /E /XF *.gguf *.part *.log settings.json capture_selection.py translate-wails.exe /XD __pycache__ > $null
    }

    # Ensure empty models directory structure is preserved if models dir existed
    $targetModelsDir = Join-Path $tempStaging "models\Hy-MT2-1.8B-GGUF"
    if (-not (Test-Path $targetModelsDir)) {
        New-Item -ItemType Directory -Path $targetModelsDir -Force | Out-Null
    }

    Write-Host "[INFO] Compressing archive via .NET standard ZipFile..." -ForegroundColor Cyan
    [System.IO.Compression.ZipFile]::CreateFromDirectory(
        $tempStaging,
        $zipPath,
        [System.IO.Compression.CompressionLevel]::Optimal,
        $false
    )
} finally {
    if (Test-Path $tempStaging) {
        Remove-Item -Path $tempStaging -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 5. Report package size
if (Test-Path $zipPath) {
    $fileItem = Get-Item $zipPath
    $sizeMB = [math]::Round($fileItem.Length / 1MB, 2)
    Write-Host ""
    Write-Host "===================================================" -ForegroundColor Green
    Write-Host "  Package Created Successfully!" -ForegroundColor Green
    Write-Host "  Version:       $version" -ForegroundColor White
    Write-Host "  Archive:       $zipPath" -ForegroundColor White
    Write-Host "  Size:          $sizeMB MB" -ForegroundColor White
    Write-Host "  Include Model: $IncludeModel" -ForegroundColor White
    Write-Host "===================================================" -ForegroundColor Green
    Write-Host ""
} else {
    Write-Host "[ERROR] Output zip file was not created!" -ForegroundColor Red
    exit 1
}
