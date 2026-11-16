[CmdletBinding()]
param(
    [string]$WheelDir = ".wheelhouse",
    [ValidateSet("cpu", "vulkan")]
    [string]$Variant = "cpu",
    [string]$Version = "",
    [string]$ReleaseTag = "",
    [string]$RequirementsFile = "backend/requirements.txt",
    [string]$BuildRequirementsFile = "backend/requirements-build.txt",
    [string]$PythonExe = "python",
    [switch]$Install,
    [switch]$Rebuild,
    [switch]$VerifyChecksums,
    [switch]$TryUpstreamBinary
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$rootDir = (Get-Item $scriptDir).Parent.FullName

function Resolve-RepoPath([string]$PathValue) {
    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return $PathValue
    }
    return Join-Path $rootDir $PathValue
}

function Get-LlamaRequirementSpec([string]$ReqPath, [string]$ExplicitVersion) {
    if ($ExplicitVersion -and $ExplicitVersion.Trim() -ne "") {
        $v = $ExplicitVersion.Trim()
        if ($v -match '^llama[-_]cpp[-_]python') {
            return $v
        }
        return "llama-cpp-python==$v"
    }
    if (-not (Test-Path $ReqPath)) {
        return "llama-cpp-python>=0.3.0"
    }
    foreach ($line in Get-Content -Path $ReqPath -Encoding UTF8) {
        $trimmed = $line.Trim()
        if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
            continue
        }
        $clean = ($trimmed -split "#", 2)[0].Trim()
        if ($clean -match '^llama[-_]cpp[-_]python(\b|\[|[=<>!~])') {
            return $clean
        }
    }
    return "llama-cpp-python>=0.3.0"
}

function Test-WheelChecksums([string]$TargetDir) {
    $sumFile = Join-Path $TargetDir "SHA256SUMS.txt"
    if (-not (Test-Path $sumFile)) {
        throw "SHA256SUMS.txt not found in $TargetDir"
    }
    $lines = Get-Content -Path $sumFile -Encoding UTF8 | Where-Object { $_.Trim() -ne "" }
    if (-not $lines) {
        throw "SHA256SUMS.txt is empty in $TargetDir"
    }
    foreach ($line in $lines) {
        if ($line -notmatch '^([0-9a-fA-F]{64})\s+\*?(.+)$') {
            throw "Malformed checksum line in SHA256SUMS.txt: $line"
        }
        $expected = $Matches[1].ToLowerInvariant()
        $fileName = [System.IO.Path]::GetFileName($Matches[2].Trim())
        $filePath = Join-Path $TargetDir $fileName
        if (-not (Test-Path $filePath)) {
            throw "Checksum target missing: $filePath"
        }
        $actual = (Get-FileHash -Path $filePath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $expected) {
            throw "SHA256 mismatch for ${fileName}: expected $expected, got $actual"
        }
    }
}

function Write-WheelChecksums([string]$TargetDir) {
    $wheels = Get-ChildItem -Path $TargetDir -Filter "*.whl" | Sort-Object Name
    $sumFile = Join-Path $TargetDir "SHA256SUMS.txt"
    $lines = foreach ($wheel in $wheels) {
        $hash = (Get-FileHash -Path $wheel.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash *$($wheel.Name)"
    }
    Set-Content -Path $sumFile -Value $lines -Encoding UTF8
}

function Test-CachedWheelCompatible([string]$PyCmd, [string]$TargetDir, [string]$Spec) {
    if (-not (Test-Path $TargetDir)) {
        return $false
    }
    $wheels = Get-ChildItem -Path $TargetDir -Filter "llama_cpp_python-*.whl" -ErrorAction SilentlyContinue
    if (-not $wheels) {
        return $false
    }
    & $PyCmd -m pip install --no-deps --no-index --find-links $TargetDir --dry-run --quiet $Spec 2>&1 | Out-Null
    return ($LASTEXITCODE -eq 0)
}

function Try-DownloadReleaseWheels([string]$Tag, [string]$TargetDir, [string]$PyCmd, [string]$Spec) {
    if (-not $Tag -or $Tag.Trim() -eq "") {
        return $false
    }
    $ghCmd = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $ghCmd) {
        Write-Host "[llama-wheel] gh CLI unavailable; skipping GitHub Release wheel lookup."
        return $false
    }
    if (-not $env:GH_TOKEN -and -not $env:GITHUB_TOKEN) {
        Write-Host "[llama-wheel] GH_TOKEN unset; skipping GitHub Release wheel lookup."
        return $false
    }

    Write-Host "[llama-wheel] Checking GitHub Release '$Tag' for precompiled wheels..."
    Get-ChildItem -Path $TargetDir -Include "llama_cpp_python-*.whl", "SHA256SUMS.txt" -File -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue

    $previousEAP = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & gh release download $Tag --pattern "*.whl" --pattern "SHA256SUMS.txt" --dir $TargetDir --clobber 2>&1 | Out-Null
        $ghExit = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousEAP
    }

    if ($ghExit -ne 0) {
        Write-Host "[llama-wheel] No precompiled wheel release asset found for tag '$Tag'."
        Get-ChildItem -Path $TargetDir -Include "llama_cpp_python-*.whl", "SHA256SUMS.txt" -File -ErrorAction SilentlyContinue |
            Remove-Item -Force -ErrorAction SilentlyContinue
        return $false
    }

    try {
        Test-WheelChecksums -TargetDir $TargetDir
    } catch {
        Write-Warning "[llama-wheel] Downloaded wheel checksum verification failed ($_); discarding and rebuilding cleanly."
        Get-ChildItem -Path $TargetDir -Include "llama_cpp_python-*.whl", "SHA256SUMS.txt" -File -ErrorAction SilentlyContinue |
            Remove-Item -Force -ErrorAction SilentlyContinue
        return $false
    }

    return (Test-CachedWheelCompatible -PyCmd $PyCmd -TargetDir $TargetDir -Spec $Spec)
}

$wheelPath = Resolve-RepoPath $WheelDir
$reqPath = Resolve-RepoPath $RequirementsFile
$buildReqPath = Resolve-RepoPath $BuildRequirementsFile

New-Item -ItemType Directory -Path $wheelPath -Force | Out-Null

$targetSpec = Get-LlamaRequirementSpec -ReqPath $reqPath -ExplicitVersion $Version
Write-Host "[llama-wheel] Target requirement: $targetSpec (variant=$Variant, dir=$wheelPath)"

$hasCompatible = $false
if (-not $Rebuild) {
    $hasCompatible = Test-CachedWheelCompatible -PyCmd $PythonExe -TargetDir $wheelPath -Spec $targetSpec
    if ($hasCompatible -and ($VerifyChecksums -or (Test-Path (Join-Path $wheelPath "SHA256SUMS.txt")))) {
        Write-Host "[llama-wheel] Verifying cached wheel SHA256SUMS.txt..."
        try {
            Test-WheelChecksums -TargetDir $wheelPath
        } catch {
            Write-Warning "[llama-wheel] Cached checksum invalid ($_); refreshing wheelhouse."
            Get-ChildItem -Path $wheelPath -Include "llama_cpp_python-*.whl", "SHA256SUMS.txt" -File -ErrorAction SilentlyContinue |
                Remove-Item -Force -ErrorAction SilentlyContinue
            $hasCompatible = $false
        }
    }
}

if (-not $hasCompatible -and -not $Rebuild -and $ReleaseTag) {
    $hasCompatible = Try-DownloadReleaseWheels -Tag $ReleaseTag -TargetDir $wheelPath -PyCmd $PythonExe -Spec $targetSpec
}

if (-not $hasCompatible -and $TryUpstreamBinary -and -not $Rebuild -and $Variant -eq "cpu") {
    Write-Host "[llama-wheel] Trying upstream CPU prebuilt wheel..."
    $previousEAP = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & $PythonExe -m pip wheel --no-deps --only-binary=llama-cpp-python --wheel-dir $wheelPath --extra-index-url https://abetlen.github.io/llama-cpp-python/whl/cpu $targetSpec 2>&1 | Out-Host
        $upstreamExit = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousEAP
    }
    if ($upstreamExit -eq 0) {
        $hasCompatible = Test-CachedWheelCompatible -PyCmd $PythonExe -TargetDir $wheelPath -Spec $targetSpec
    } else {
        Write-Host "[llama-wheel] No matching upstream binary wheel; falling back to source build."
    }
}

if (-not $hasCompatible) {
    Write-Host "[llama-wheel] Building llama-cpp-python wheel from source ($Variant)..."
    if ($Variant -eq "vulkan" -and -not $env:VULKAN_SDK) {
        throw "VULKAN_SDK environment variable must be set to compile Vulkan wheel."
    }

    & $PythonExe -m pip install --upgrade ninja cmake scikit-build-core
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to install CMake/Ninja build tooling"
    }

    $cpuCount = [Math]::Max(1, [Environment]::ProcessorCount)
    $env:CMAKE_BUILD_PARALLEL_LEVEL = "$cpuCount"
    $env:FORCE_CMAKE = "1"
    if ($Variant -eq "vulkan") {
        $env:CMAKE_ARGS = "-DGGML_VULKAN=ON -DGGML_NATIVE=OFF"
    } else {
        $env:CMAKE_ARGS = "-DGGML_NATIVE=OFF"
    }

    Get-ChildItem -Path $wheelPath -Include "llama_cpp_python-*.whl", "SHA256SUMS.txt" -File -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue

    & $PythonExe -m pip wheel --no-deps --no-binary=llama-cpp-python --wheel-dir $wheelPath $targetSpec
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to build llama-cpp-python wheel"
    }
}

Write-WheelChecksums -TargetDir $wheelPath
Get-ChildItem -Path $wheelPath -Filter "llama_cpp_python-*.whl" | ForEach-Object {
    Write-Host "[llama-wheel] Ready: $($_.Name) ($([Math]::Round($_.Length / 1MB, 2)) MB)"
}

if ($Install) {
    Write-Host "[llama-wheel] Installing prebuilt llama-cpp-python from $wheelPath..."
    & $PythonExe -m pip install --no-deps --no-index --find-links $wheelPath $targetSpec
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to install llama-cpp-python from prebuilt wheelhouse"
    }

    if (Test-Path $buildReqPath) {
        Write-Host "[llama-wheel] Installing remaining build dependencies from $buildReqPath..."
        & $PythonExe -m pip install --prefer-binary --find-links $wheelPath -r $buildReqPath
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to install build requirements"
        }
    }

    & $PythonExe -c "import llama_cpp; assert hasattr(llama_cpp, 'Llama'); print('[llama-wheel] Verified:', llama_cpp.__version__, '| gpu_offload:', getattr(llama_cpp, 'llama_supports_gpu_offload', lambda: False)())"
    if ($LASTEXITCODE -ne 0) {
        throw "llama_cpp import verification failed"
    }
}

exit 0
