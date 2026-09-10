[CmdletBinding()]
param(
    [switch]$Stop,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSCommandPath
$backendDevPort = if ($env:BACKEND_DEV_PORT) { $env:BACKEND_DEV_PORT } else { "8383" }
Set-Location -LiteralPath $projectRoot

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker bulunamadi. Docker Desktop'i kurup yeniden deneyin."
}

& docker info *> $null
if ($LASTEXITCODE -ne 0) {
    throw "Docker calismiyor. Docker Desktop'i baslatip yeniden deneyin."
}

if ($Stop) {
    & docker compose -f docker-compose.yml -f docker-compose.dev.yml down
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

$notificationTokenMatch = [regex]::Match($envContent, "(?m)^NOTIFICATION_SERVICE_TOKEN=(.*)$")
if (-not $notificationTokenMatch.Success -or
    $notificationTokenMatch.Groups[1].Value.Trim().Length -lt 32 -or
    $notificationTokenMatch.Groups[1].Value.Contains("replace-with")) {
    $randomBytes = New-Object byte[] 32
    $randomGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $randomGenerator.GetBytes($randomBytes)
        $notificationToken = [Convert]::ToBase64String($randomBytes)
    }
    finally {
        $randomGenerator.Dispose()
    }

    $notificationTokenLine = "NOTIFICATION_SERVICE_TOKEN=$notificationToken"
    if ($notificationTokenMatch.Success) {
        $envContent = [regex]::Replace(
            $envContent,
            "(?m)^NOTIFICATION_SERVICE_TOKEN=.*$",
            [Text.RegularExpressions.MatchEvaluator]{ param($match) $notificationTokenLine },
            1
        )
    }
    else {
        if ($envContent.Length -gt 0 -and -not $envContent.EndsWith("`n")) {
            $envContent += [Environment]::NewLine
        }
        $envContent += $notificationTokenLine + [Environment]::NewLine
    }
    [IO.File]::WriteAllText($envPath, $envContent, (New-Object Text.UTF8Encoding($false)))
    Write-Host "Guvenli NOTIFICATION_SERVICE_TOKEN otomatik olusturuldu." -ForegroundColor Yellow
}

$rabbitPasswordMatch = [regex]::Match($envContent, "(?m)^RABBITMQ_PASSWORD=(.*)$")
if (-not $rabbitPasswordMatch.Success -or
    $rabbitPasswordMatch.Groups[1].Value.Trim().Length -lt 32 -or
    $rabbitPasswordMatch.Groups[1].Value.Contains("replace-with")) {
    $randomBytes = New-Object byte[] 32
    $randomGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $randomGenerator.GetBytes($randomBytes)
        # RabbitMQ credentials are embedded in an AMQP URL in Compose, so use
        # URL-safe Base64 rather than characters such as / or +.
        $rabbitPassword = [Convert]::ToBase64String($randomBytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
    }
    finally {
        $randomGenerator.Dispose()
    }

    $rabbitPasswordLine = "RABBITMQ_PASSWORD=$rabbitPassword"
    if ($rabbitPasswordMatch.Success) {
        $envContent = [regex]::Replace(
            $envContent,
            "(?m)^RABBITMQ_PASSWORD=.*$",
            [Text.RegularExpressions.MatchEvaluator]{ param($match) $rabbitPasswordLine },
            1
        )
    }
    else {
        if ($envContent.Length -gt 0 -and -not $envContent.EndsWith("`n")) {
            $envContent += [Environment]::NewLine
        }
        $envContent += $rabbitPasswordLine + [Environment]::NewLine
    }
    [IO.File]::WriteAllText($envPath, $envContent, (New-Object Text.UTF8Encoding($false)))
    Write-Host "Guvenli RABBITMQ_PASSWORD otomatik olusturuldu." -ForegroundColor Yellow
}

Write-Host "PostgreSQL, RabbitMQ, MailHog ve Notification servisi baslatiliyor..." -ForegroundColor Cyan
& docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build --wait --wait-timeout 120 postgres rabbitmq mailhog notification-service
if ($LASTEXITCODE -ne 0) {
    & docker compose ps -a
    & docker compose logs --tail 80
    throw "Gelistirme bagimliliklari baslatilamadi. Yukaridaki Docker loglarini kontrol edin."
}

if (-not $NoBuild) {
    Write-Host "Linux backend binary Mage ile derleniyor..." -ForegroundColor Cyan
    Push-Location -LiteralPath (Join-Path $projectRoot "backend")
    try {
        $env:MAGEFILE_CACHE = Join-Path $projectRoot "backend/.magecache"
        try {
            & mage -v build:linux
        }
        catch {
            Write-Host "Yerel mage calistirilamadi; ayni Mage surumu go run ile baslatiliyor..." -ForegroundColor Yellow
            & go run github.com/magefile/mage@v1.17.2 -v build:linux
        }
        if ($LASTEXITCODE -ne 0) {
            throw "Backend binary derlenemedi."
        }
    }
    finally {
        Pop-Location
    }
}

$binaryPath = Join-Path $projectRoot "backend/bin/linux/ledger"
if (-not (Test-Path -LiteralPath $binaryPath)) {
    throw "Backend binary bulunamadi: $binaryPath. .\start.ps1 komutunu -NoBuild olmadan calistirin."
}

Write-Host "Veritabani migration'lari uygulanıyor..." -ForegroundColor Cyan
& docker compose -f docker-compose.yml -f docker-compose.dev.yml run --rm migrate
if ($LASTEXITCODE -ne 0) {
    throw "Veritabani migration'lari uygulanamadi."
}

Write-Host "Linux backend binary container icinde baslatiliyor..." -ForegroundColor Cyan
& docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --wait --wait-timeout 120 backend-dev
if ($LASTEXITCODE -ne 0) {
    & docker compose -f docker-compose.yml -f docker-compose.dev.yml logs --tail 80 backend-dev
    throw "Backend baslatilamadi."
}

$frontendPath = Join-Path $projectRoot "frontend"
if (-not (Test-Path -LiteralPath (Join-Path $frontendPath "node_modules/next/package.json"))) {
    Write-Host "Frontend npm bagimliliklari kuruluyor..." -ForegroundColor Cyan
    Push-Location -LiteralPath $frontendPath
    try {
        & npm install --no-package-lock
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend bagimliliklari kurulamadi."
        }
    }
    finally {
        Pop-Location
    }
}

Write-Host ""
Write-Host "Backend ve PostgreSQL hazir; frontend gelistirme sunucusu baslatiliyor." -ForegroundColor Green
Write-Host "Frontend : http://localhost:3000"
Write-Host "Backend  : http://localhost:$backendDevPort"
Write-Host "Swagger  : http://localhost:$backendDevPort/swagger/index.html"
Write-Host "MailHog  : http://localhost:8425"
Write-Host "RabbitMQ : http://localhost:15672"
Write-Host "Notify   : http://localhost:8490/health"
Write-Host "Database : localhost:5433"
Write-Host "Durdurmak icin frontend terminalinde Ctrl+C, ardindan: .\start.ps1 -Stop"
Write-Host ""

Push-Location -LiteralPath $frontendPath
try {
    $env:BACKEND_API_URL = "http://localhost:$backendDevPort"
    & npm run dev
}
finally {
    Pop-Location
}
