[CmdletBinding()]
param(
    [switch]$Stop,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSCommandPath
Set-Location -LiteralPath $projectRoot

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker bulunamadi. Docker Desktop'i kurup yeniden deneyin."
}

& docker info *> $null
if ($LASTEXITCODE -ne 0) {
    throw "Docker calismiyor. Docker Desktop'i baslatip yeniden deneyin."
}

if ($Stop) {
    & docker compose down
    exit $LASTEXITCODE
}

$envPath = Join-Path $projectRoot ".env"
$envExamplePath = Join-Path $projectRoot ".env.example"

if (-not (Test-Path -LiteralPath $envPath)) {
    Copy-Item -LiteralPath $envExamplePath -Destination $envPath
    Write-Host ".env, .env.example dosyasindan olusturuldu." -ForegroundColor Yellow
}

$envContent = [IO.File]::ReadAllText($envPath)
$jwtMatch = [regex]::Match($envContent, "(?m)^JWT_SECRET=(.*)$")

if (-not $jwtMatch.Success -or $jwtMatch.Groups[1].Value.Trim().Length -lt 32) {
    $randomBytes = New-Object byte[] 32
    $randomGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $randomGenerator.GetBytes($randomBytes)
        $jwtSecret = [Convert]::ToBase64String($randomBytes)
    }
    finally {
        $randomGenerator.Dispose()
    }

    $jwtLine = "JWT_SECRET=$jwtSecret"
    if ($jwtMatch.Success) {
        $envContent = [regex]::Replace(
            $envContent,
            "(?m)^JWT_SECRET=.*$",
            [Text.RegularExpressions.MatchEvaluator]{ param($match) $jwtLine },
            1
        )
    }
    else {
        if ($envContent.Length -gt 0 -and -not $envContent.EndsWith("`n")) {
            $envContent += [Environment]::NewLine
        }
        $envContent += $jwtLine + [Environment]::NewLine
    }

    [IO.File]::WriteAllText(
        $envPath,
        $envContent,
        (New-Object Text.UTF8Encoding($false))
    )
    Write-Host "Guvenli JWT_SECRET otomatik olusturuldu." -ForegroundColor Yellow
}

$dockerArguments = @("compose", "up", "-d")
if (-not $NoBuild) {
    $dockerArguments += "--build"
}
$dockerArguments += @("--wait", "--wait-timeout", "240")

Write-Host "Database, backend ve frontend baslatiliyor..." -ForegroundColor Cyan
& docker @dockerArguments
if ($LASTEXITCODE -ne 0) {
    & docker compose ps -a
    & docker compose logs --tail 80
    throw "Servisler baslatilamadi. Yukaridaki Docker loglarini kontrol edin."
}

Write-Host ""
Write-Host "Tum servisler hazir." -ForegroundColor Green
Write-Host "Frontend : http://localhost:3000"
Write-Host "Backend  : http://localhost:8080"
Write-Host "Swagger  : http://localhost:8080/swagger/index.html"
Write-Host "Database : localhost:5433"
Write-Host ""
Write-Host "Durdurmak icin: .\start.ps1 -Stop"

& docker compose ps
