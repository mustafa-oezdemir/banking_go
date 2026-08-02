[CmdletBinding()]
param(
    [string]$BaseUrl = "http://localhost:8080",
    [switch]$AllowRemote
)

$ErrorActionPreference = "Stop"
$securityTarget = [Uri]$BaseUrl
$isLocalTarget = $securityTarget.IsLoopback -or $securityTarget.Host -eq "localhost"
if (-not $isLocalTarget -and -not $AllowRemote) {
    throw "Safety stop: remote targets require -AllowRemote and explicit authorization."
}

$securityBaseUrl = $BaseUrl.TrimEnd("/")
$securityOrigin = $securityTarget.GetLeftPart([UriPartial]::Authority)
$securityCsrfHeaders = @{
    "X-CSRF-Protection" = "1"
    "Origin" = $securityOrigin
    "Sec-Fetch-Site" = "same-origin"
}
$securityResults = [System.Collections.Generic.List[object]]::new()

function Invoke-SecurityRequest {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body,
        [string]$ContentType = "application/json",
        [hashtable]$Headers,
        [object]$WebSession
    )

    $requestParameters = @{
        Uri = "$securityBaseUrl$Path"
        Method = $Method
        UseBasicParsing = $true
        TimeoutSec = 10
    }
    if ($null -ne $Body) {
        $requestParameters.Body = $Body
        $requestParameters.ContentType = $ContentType
    }
    if ($Headers) {
        $requestParameters.Headers = $Headers
    }
    if ($WebSession) {
        $requestParameters.WebSession = $WebSession
    }

    try {
        $response = Invoke-WebRequest @requestParameters
        return [pscustomobject]@{
            Status = [int]$response.StatusCode
            Headers = $response.Headers
            Content = $response.Content
        }
    }
    catch {
        if (-not $_.Exception.Response) {
            throw
        }
        $response = $_.Exception.Response
        $content = ""
        $statusCode = [int]$response.StatusCode
        # Windows PowerShell 5 can block while reading an early 413 response if it
        # is still unwinding the oversized upload. The status is the assertion.
        $responseStream = if ($statusCode -eq 413) { $null } else { $response.GetResponseStream() }
        if ($null -ne $responseStream) {
            $reader = [System.IO.StreamReader]::new($responseStream)
            try { $content = $reader.ReadToEnd() } finally { $reader.Dispose() }
        }
        return [pscustomobject]@{
            Status = $statusCode
            Headers = $response.Headers
            Content = $content
        }
    }
}

function Assert-SecurityStatus {
    param(
        [string]$Name,
        [object]$Response,
        [int[]]$Expected
    )
    $actualStatus = if ($null -ne $Response.Status) { [int]$Response.Status } else { [int]$Response.StatusCode }
    $passed = $Expected -contains $actualStatus
    $securityResults.Add([pscustomobject]@{
        Test = $Name
        Status = $actualStatus
        Result = if ($passed) { "PASS" } else { "FAIL" }
    })
    if (-not $passed) {
        throw "$Name failed: expected $($Expected -join ', '), got $actualStatus."
    }
}

$health = Invoke-SecurityRequest -Method GET -Path "/health"
Assert-SecurityStatus -Name "Health endpoint" -Response $health -Expected 200
foreach ($requiredHeader in @("X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy")) {
    if (-not $health.Headers[$requiredHeader]) {
        throw "Security header missing: $requiredHeader"
    }
}

$unauthorized = Invoke-SecurityRequest -Method GET -Path "/accounts/00000000-0000-0000-0000-000000000000"
Assert-SecurityStatus -Name "Unauthenticated account access" -Response $unauthorized -Expected 401

$wrongContentType = Invoke-SecurityRequest -Method POST -Path "/login" -Body "email=user@example.com" -ContentType "application/x-www-form-urlencoded"
Assert-SecurityStatus -Name "Content-Type enforcement" -Response $wrongContentType -Expected 415

$tcpClient = [Net.Sockets.TcpClient]::new()
$targetPort = if ($securityTarget.IsDefaultPort) {
    if ($securityTarget.Scheme -eq "https") { 443 } else { 80 }
} else {
    $securityTarget.Port
}
try {
    $tcpClient.Connect($securityTarget.Host, $targetPort)
    $requestStream = $tcpClient.GetStream()
    if ($securityTarget.Scheme -eq "https") {
        $tlsStream = [Net.Security.SslStream]::new($requestStream, $false)
        $tlsStream.AuthenticateAsClient($securityTarget.Host)
        $requestStream = $tlsStream
    }
    $requestPath = "$($securityTarget.AbsolutePath.TrimEnd('/'))/login"
    $rawRequest = "POST $requestPath HTTP/1.1`r`nHost: $($securityTarget.Authority)`r`nContent-Type: application/json`r`nContent-Length: 1048578`r`nConnection: close`r`n`r`n"
    $rawRequestBytes = [Text.Encoding]::ASCII.GetBytes($rawRequest)
    $requestStream.Write($rawRequestBytes, 0, $rawRequestBytes.Length)
    $responseReader = [IO.StreamReader]::new($requestStream)
    $statusLine = $responseReader.ReadLine()
    $oversized = [pscustomobject]@{ Status = [int]($statusLine.Split(' ')[1]) }
}
finally {
    if ($responseReader) { $responseReader.Dispose() }
    if ($tlsStream) { $tlsStream.Dispose() }
    $tcpClient.Dispose()
}
Assert-SecurityStatus -Name "Request size limit" -Response $oversized -Expected 413

$weakRegistrationBody = @{ email = "weak_$([Guid]::NewGuid().ToString('N'))@example.com"; password = "password" } | ConvertTo-Json -Compress
$weakRegistration = Invoke-SecurityRequest -Method POST -Path "/register" -Body $weakRegistrationBody
Assert-SecurityStatus -Name "Weak password rejection" -Response $weakRegistration -Expected 400

$injectionLoginBody = @{ email = "' OR 1=1 --@example.com"; password = "correct horse battery staple" } | ConvertTo-Json -Compress
$injectionLogin = Invoke-SecurityRequest -Method POST -Path "/login" -Body $injectionLoginBody
Assert-SecurityStatus -Name "SQL injection login payload" -Response $injectionLogin -Expected @(400, 401)

$userAPassword = "Correct Horse Battery Staple 123!"
$userAEmail = "security_a_$([Guid]::NewGuid().ToString('N'))@example.com"
$userABody = @{ email = $userAEmail; password = $userAPassword } | ConvertTo-Json -Compress
$registerAParameters = @{
    Uri = "$securityBaseUrl/register"
    Method = "POST"
    Body = $userABody
    ContentType = "application/json"
    SessionVariable = "sessionA"
    UseBasicParsing = $true
    TimeoutSec = 10
}
$registerA = Invoke-WebRequest @registerAParameters
Assert-SecurityStatus -Name "Secure user registration" -Response $registerA -Expected 201
$setCookieHeader = [string]$registerA.Headers["Set-Cookie"]
if ($setCookieHeader -notmatch "HttpOnly" -or $setCookieHeader -notmatch "SameSite=Strict") {
    throw "Session cookie is missing HttpOnly or SameSite=Strict."
}

$missingCsrf = Invoke-SecurityRequest -Method POST -Path "/accounts" -Body (@{ name = "No CSRF" } | ConvertTo-Json -Compress) -WebSession $sessionA
Assert-SecurityStatus -Name "Missing CSRF header" -Response $missingCsrf -Expected 403

$xssName = '<img src=x onerror=alert(1)>'
$xssAccountBody = @{ name = $xssName } | ConvertTo-Json -Compress
$xssAccount = Invoke-SecurityRequest -Method POST -Path "/accounts" -Body $xssAccountBody -Headers $securityCsrfHeaders -WebSession $sessionA
Assert-SecurityStatus -Name "Stored XSS payload handling" -Response $xssAccount -Expected 201
$xssAccountData = $xssAccount.Content | ConvertFrom-Json
if ($xssAccountData.name -ne $xssName) {
    throw "Stored input changed unexpectedly; verify contextual React output encoding."
}

$crossSiteHeaders = @{
    "X-CSRF-Protection" = "1"
    "Origin" = "https://attacker.invalid"
    "Sec-Fetch-Site" = "cross-site"
}
$crossSite = Invoke-SecurityRequest -Method POST -Path "/accounts" -Body (@{ name = "Cross Site" } | ConvertTo-Json -Compress) -Headers $crossSiteHeaders -WebSession $sessionA
Assert-SecurityStatus -Name "Cross-site CSRF" -Response $crossSite -Expected 403

$userBEmail = "security_b_$([Guid]::NewGuid().ToString('N'))@example.com"
$userBBody = @{ email = $userBEmail; password = $userAPassword } | ConvertTo-Json -Compress
$registerBParameters = @{
    Uri = "$securityBaseUrl/register"
    Method = "POST"
    Body = $userBBody
    ContentType = "application/json"
    SessionVariable = "sessionB"
    UseBasicParsing = $true
    TimeoutSec = 10
}
$registerB = Invoke-WebRequest @registerBParameters
Assert-SecurityStatus -Name "Second security user" -Response $registerB -Expected 201

$idor = Invoke-SecurityRequest -Method GET -Path "/accounts/$($xssAccountData.id)" -WebSession $sessionB
Assert-SecurityStatus -Name "Cross-user IDOR protection" -Response $idor -Expected 403

$deleteXss = Invoke-SecurityRequest -Method DELETE -Path "/accounts/$($xssAccountData.id)" -Headers $securityCsrfHeaders -WebSession $sessionA
Assert-SecurityStatus -Name "Security test cleanup" -Response $deleteXss -Expected 200

$securityResults | Format-Table -AutoSize
Write-Host "Security smoke tests passed against $securityBaseUrl" -ForegroundColor Green
