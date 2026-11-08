param(
    [string]$Server = $env:HEPHAESTUS_SERVER_URL,
    [ValidateSet('install', 'update', 'run')][string]$Action = 'install',
    [string]$InstallDir = '',
    [string]$Version = '',
    [string]$ReleaseBase = '',
    [string]$ConfigFile = '',
    [switch]$NoStart
)
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$App = 'hephaestus-scan-agent.exe'
if (!$InstallDir) {
    if (Test-Path (Join-Path $PSScriptRoot '.installed-version')) { $InstallDir = $PSScriptRoot }
    else { $InstallDir = Join-Path $PWD.Path 'hephaestus-agent' }
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$Binary = Join-Path $InstallDir $App

function Write-Log([string]$Message) { Write-Host "[hephaestus-agent] $Message" }
function Invoke-AgentDownload([string]$Uri, [string]$Resource, [string]$OutFile = '', [int]$TimeoutSec = 30) {
    $Options = @{ UseBasicParsing = $true; Uri = $Uri; TimeoutSec = $TimeoutSec }
    if ($OutFile) { $Options.OutFile = $OutFile }
    try { return Invoke-WebRequest @Options }
    catch {
        $Original = $_.Exception.Message
        $Status = $null
        $TimedOut = $false
        $Interrupted = $false
        for ($Cause = $_.Exception; $Cause; $Cause = $Cause.InnerException) {
            if ($Cause.Response -and $Cause.Response.StatusCode) { $Status = [int]$Cause.Response.StatusCode }
            if ($Cause -is [TimeoutException] -or $Cause -is [OperationCanceledException]) { $TimedOut = $true }
            if ($Cause -is [Net.WebException]) {
                if ($Cause.Status -eq [Net.WebExceptionStatus]::Timeout) { $TimedOut = $true }
                if ($Cause.Status -in @([Net.WebExceptionStatus]::ReceiveFailure, [Net.WebExceptionStatus]::SendFailure, [Net.WebExceptionStatus]::ConnectionClosed)) { $Interrupted = $true }
            }
        }
        if ($Status -eq 404) { $Detail = 'HTTP 404: requested file or endpoint was not found; check the server/version and release address.' }
        elseif ($Status) { $Detail = "Server returned HTTP $Status; retry or check access to the download server." }
        elseif ($TimedOut) { $Detail = 'Request timed out; check network connectivity and proxy settings, then retry.' }
        elseif ($Interrupted) { $Detail = 'Connection interrupted or reset; check network connectivity and proxy settings, then retry.' }
        else { $Detail = 'Download failed; check network connectivity, DNS, proxy settings, directory permissions and disk space.' }
        throw "${Resource}: $Detail Original error: $Original"
    }
}
function Run-Node {
    if (!(Test-Path $Binary) -or !(Test-Path (Join-Path $InstallDir 'config.yaml'))) { throw 'Install and configure the node first.' }
    $env:HEPHAESTUS_CONFIG_DIR = $InstallDir
    $env:HEPHAESTUS_RESOURCE_DIR = Join-Path $InstallDir 'config'
    Push-Location $InstallDir
    $Supervisor = $null
    try {
        $HelpOutput = & $Binary help
        if (($HelpOutput -join "`n") -match '(?m)^  supervise') {
            $Supervisor = Join-Path $InstallDir ('.agent-supervisor-' + [Guid]::NewGuid().ToString('N') + '.exe')
            Copy-Item -LiteralPath $Binary -Destination $Supervisor
            & $Supervisor supervise --binary $Binary
        } else { & $Binary }
        $script:NodeExitCode = $LASTEXITCODE
    } finally {
        if ($Supervisor) { Remove-Item -LiteralPath $Supervisor -Force -ErrorAction SilentlyContinue }
        Pop-Location
    }
}
if ($Action -eq 'run') { Run-Node; exit $NodeExitCode }

# FileShare.None serializes installers; Windows releases the lock on a crash.
$Lock = $null
$TempDir = $null
try {
    $Lock = [IO.File]::Open((Join-Path $InstallDir '.install-lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $Running = Get-Process -Name 'hephaestus-scan-agent' -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $Binary }
    if ($Running) { throw 'Stop this node with Ctrl+C in its running console before updating. The installer does not force-stop tasks.' }
    $ServerFile = Join-Path $InstallDir '.server-url'
    if (!$Server -and (Test-Path $ServerFile)) { $Server = [IO.File]::ReadAllText($ServerFile).Trim() }
    if (!$Server) { $Server = Read-Host 'Hephaestus server URL (reachable from this node)' }
    if ($Server -notmatch '^https?://\S+$') { throw 'A valid http(s) server URL is required.' }
    $Server = $Server.TrimEnd('/')
    if (!$ReleaseBase -and (Test-Path (Join-Path $InstallDir '.release-base'))) {
        $ReleaseBase = [IO.File]::ReadAllText((Join-Path $InstallDir '.release-base')).Trim()
    }
    if (!$ReleaseBase) { $ReleaseBase = 'https://github.com/qiwentaidi/Hephaestus-Agent/releases/download' }
    if ($ReleaseBase -notmatch '^https?://\S+$') { throw 'Invalid release mirror URL.' }
    $ReleaseBase = $ReleaseBase.TrimEnd('/')
    $NativeArch = $env:PROCESSOR_ARCHITEW6432
    if (!$NativeArch) { $NativeArch = $env:PROCESSOR_ARCHITECTURE }
    if ($NativeArch -ne 'AMD64') { throw 'This release supports Windows amd64 only. ARM64 requires a separately supported package.' }
    $Recommended = (Invoke-AgentDownload -Uri "$Server/api/v1/agent/release?format=text" -Resource 'Compatible version query').Content.Trim()
    if ($Recommended -notmatch '^v\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?$') { throw 'Invalid compatible version response.' }
    if ($Version -and $Version -ne $Recommended) { throw "Version $Version is incompatible; the server recommends $Recommended." }
    $Version = $Recommended
    $Package = 'hephaestus-agent-windows-amd64.zip'
    $TempDir = Join-Path ([IO.Path]::GetTempPath()) ('hephaestus-agent-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $TempDir | Out-Null
    $Archive = Join-Path $TempDir 'package.zip'
    Write-Log "Downloading $Version (Windows/amd64)"
    Invoke-AgentDownload -Uri "$ReleaseBase/$Version/$Package" -Resource "Release $Version package $Package" -OutFile $Archive -TimeoutSec 300
    $Checksums = (Invoke-AgentDownload -Uri "$ReleaseBase/$Version/SHA256SUMS.txt" -Resource "Release $Version checksum file").Content
    $MatchesForPackage = @($Checksums -split '\r?\n' | Where-Object { $_ -match ('^[a-fA-F0-9]{64}\s+\*?' + [regex]::Escape($Package) + '$') })
    if ($MatchesForPackage.Count -ne 1) { throw 'Missing or ambiguous package checksum.' }
    $Expected = ($MatchesForPackage[0] -split '\s+')[0]
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $Archive).Hash -ne $Expected) { throw 'Checksum mismatch. The installed node has not changed.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    $StagedBinary = Join-Path $TempDir $App
    try {
        $Entry = $Zip.GetEntry($App)
        if (!$Entry) { throw 'Binary missing from package.' }
        [IO.Compression.ZipFileExtensions]::ExtractToFile($Entry, $StagedBinary, $true)
    } finally { $Zip.Dispose() }
    $Output = & $StagedBinary version
    if ($LASTEXITCODE -ne 0 -or ($Output -join "`n").Trim() -ne "Hephaestus scan-agent $Version (cli mode)") {
        throw 'Binary cannot run on this host or its version does not match the release.'
    }
    $StagedScript = Join-Path $TempDir 'install.ps1'
    Invoke-AgentDownload -Uri "$Server/api/v1/agent/install.ps1" -Resource 'Updater script' -OutFile $StagedScript
    $Tokens = $null; $ParseErrors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($StagedScript, [ref]$Tokens, [ref]$ParseErrors) | Out-Null
    if ($ParseErrors.Count) { throw 'The server supplied an invalid updater script.' }

    function Yaml-String([string]$Value) {
        if ($Value.Contains("`r") -or $Value.Contains("`n")) { throw 'Configuration values must be single-line.' }
        return "'" + $Value.Replace("'", "''") + "'"
    }
    function Ask([string]$Label, [string]$Default) {
        $Value = Read-Host "$Label [$Default]"
        if (!$Value) { return $Default }; return $Value
    }
    $ConfigPath = Join-Path $InstallDir 'config.yaml'
    if (!(Test-Path $ConfigPath)) {
        if ($ConfigFile) { Copy-Item -LiteralPath $ConfigFile -Destination $ConfigPath }
        else {
            Write-Log 'Configure the same PostgreSQL database as the main server.'
            $DbHost = Ask 'PostgreSQL host (reachable from this node)' ''
            if (!$DbHost) { throw 'PostgreSQL host is required.' }
            $DbPort = Ask 'PostgreSQL port' '5432'
            if ($DbPort -notmatch '^\d+$' -or [int]$DbPort -lt 1 -or [int]$DbPort -gt 65535) { throw 'Invalid PostgreSQL port.' }
            $DbUser = Ask 'Database user' 'postgres'
            $Secret = Read-Host 'Database password' -AsSecureString
            $SecretPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Secret)
            try { $DbPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($SecretPointer) }
            finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($SecretPointer) }
            $DbName = Ask 'Database name' 'hephaestus'
            $SslMode = Ask 'PostgreSQL sslmode (require / verify-full / disable)' 'disable'
            if ($SslMode -notin @('disable', 'require', 'verify-ca', 'verify-full')) { throw 'Invalid sslmode.' }
            $Roles = Ask 'Agent roles (scan / ai / ai,scan)' 'ai,scan'
            switch ($Roles) {
                'scan' { $RoleYaml = "['scan']" }
                'ai' { $RoleYaml = "['ai']" }
                'ai,scan' { $RoleYaml = "['ai', 'scan']" }
                'scan,ai' { $RoleYaml = "['ai', 'scan']" }
                default { throw 'Invalid Agent roles.' }
            }
            $Yaml = @"
database:
  type: postgres
  host: $(Yaml-String $DbHost)
  port: $(Yaml-String $DbPort)
  user: $(Yaml-String $DbUser)
  password: $(Yaml-String $DbPassword)
  dbname: $(Yaml-String $DbName)
  sslmode: $(Yaml-String $SslMode)
agent:
  id: ''
  roles: $RoleYaml
  aiConcurrency: 4
  scanConcurrency: 3
  server:
    address: $(Yaml-String $Server)
"@
            $DbPassword = $null
            [IO.File]::WriteAllText($ConfigPath, $Yaml, (New-Object Text.UTF8Encoding($false)))
        }
        # Remove inherited access to database credentials, retaining owner/system.
        $Acl = New-Object Security.AccessControl.FileSecurity
        $Identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
        $Acl.SetOwner($Identity)
        $Acl.SetAccessRuleProtection($true, $false)
        $Acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($Identity, 'FullControl', 'Allow')))
        $Acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule('SYSTEM', 'FullControl', 'Allow')))
        Set-Acl -LiteralPath $ConfigPath -AclObject $Acl
    }
    New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir 'config') | Out-Null
    $Previous = Join-Path $InstallDir ($App + '.previous')
    $Next = Join-Path $InstallDir ('.' + $App + '.new')
    Copy-Item -LiteralPath $StagedBinary -Destination $Next -Force
    if (Test-Path $Binary) { [IO.File]::Replace($Next, $Binary, $Previous, $true) }
    else { [IO.File]::Move($Next, $Binary) }
    $InstalledScript = Join-Path $InstallDir 'install.ps1'
    Copy-Item -LiteralPath $StagedScript -Destination $InstalledScript -Force
    [IO.File]::WriteAllText($ServerFile, $Server)
    [IO.File]::WriteAllText((Join-Path $InstallDir '.release-base'), $ReleaseBase)
    [IO.File]::WriteAllText((Join-Path $InstallDir '.installed-version'), $Version)
    foreach ($Name in @('run', 'update')) {
        $Batch = "@echo off`r`npowershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File `"%~dp0install.ps1`" -Action $Name %*`r`nexit /b %ERRORLEVEL%`r`n"
        [IO.File]::WriteAllText((Join-Path $InstallDir ($Name + '.bat')), $Batch, [Text.Encoding]::ASCII)
    }
    Write-Log "Installed $Version. Configuration and resources are preserved in $InstallDir."
    Write-Log "Next update: $InstallDir\update.bat"
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
} finally {
    if ($Lock) { $Lock.Dispose() }
    if ($TempDir) { Remove-Item -LiteralPath $TempDir -Recurse -Force -ErrorAction SilentlyContinue }
}
if (!$NoStart) { Run-Node; exit $NodeExitCode }
