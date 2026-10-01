#Requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateRange(1, 65535)][int]$Port,
    [ValidateRange(1, 86400)][int]$Timeout = 180,
    [string]$Directory = $PSScriptRoot,
    [switch]$SkipBuild,
    [switch]$Check,
    [switch]$Help
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
if ($Help) {
    Write-Host 'Komari 一键部署（需要已启动的 Docker Desktop / Linux 容器模式）'
    Write-Host '.\install.ps1 [-Port 8080] [-SkipBuild] [-Timeout 180] [-Directory PATH] [-Check]'
    Write-Host '更新源码后重复运行即可；-Check 仅检查，不启动或写文件。'
    exit 0
}
$root = (Resolve-Path -LiteralPath $Directory).Path
$envFile = Join-Path $root '.env'
$lockDir = Join-Path $root '.deploy.lock'
$envTemp = $null
$lockOwned = $false
$previousPort = [Environment]::GetEnvironmentVariable('MONITOR_PORT', 'Process')
$exitCode = 0
function Invoke-Docker {
    param([string[]]$Arguments)
    & $script:dockerExe @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Docker 命令失败（退出码 $LASTEXITCODE）。" }
}
function Invoke-Compose {
    param([string[]]$Arguments)
    Invoke-Docker -Arguments ($script:composeArgs + $Arguments)
}
try {
    foreach ($file in @('compose.yaml', 'Dockerfile', 'komari-web/package.json', 'komari-web/package-lock.json')) {
        if (-not (Test-Path -LiteralPath (Join-Path $root $file) -PathType Leaf)) {
            throw "缺少 $file，请获取完整仓库源码后运行。"
        }
    }
    if (Test-Path -LiteralPath $envFile) {
        $item = Get-Item -LiteralPath $envFile -Force
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw '.env 必须是源码目录内的普通文件，不能是目录或符号链接。'
        }
    }
    if (-not $Check) {
        if (Test-Path -LiteralPath $lockDir) { throw '已有 .deploy.lock；确认没有其他部署进程运行后再处理该锁。' }
        New-Item -ItemType Directory -Path $lockDir -ErrorAction Stop | Out-Null
        $lockOwned = $true
    }
    $dockerCommand = Get-Command docker -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $dockerCommand) { throw '请先安装并启动 Docker Desktop，选择 Linux 容器模式。' }
    $script:dockerExe = $dockerCommand.Source
    Invoke-Docker -Arguments @('compose', 'version') | Out-Null
    $osType = (Invoke-Docker -Arguments @('info', '--format', '{{.OSType}}') | Out-String).Trim()
    if ($osType -ne 'linux') { throw '请确认 Docker 已启动，并切换为 Linux 容器模式。' }
    $script:composeArgs = @('compose', '--project-directory', $root, '-f', (Join-Path $root 'compose.yaml'))
    if (-not $PSBoundParameters.ContainsKey('Port')) {
        $environment = @(Invoke-Compose -Arguments @('config', '--environment'))
        $portLine = @($environment | Where-Object { $_ -cmatch '^MONITOR_PORT=' }) | Select-Object -Last 1
        $value = if ($portLine) { $portLine.Substring('MONITOR_PORT='.Length).Trim() } else { '' }
        if (-not $value) { $value = '25774' }
        if ($value -notmatch '^\d{1,5}$') { throw 'MONITOR_PORT 必须是 1–65535 的整数。' }
        $Port = [int]$value
        if ($Port -lt 1 -or $Port -gt 65535) { throw 'MONITOR_PORT 必须是 1–65535 的整数。' }
    }
    $env:MONITOR_PORT = [string]$Port
    Invoke-Compose -Arguments @('config', '--quiet') | Out-Null
    if ($Check) {
        Write-Host "检查通过；源码: $root；端口: $Port"
    } else {
        $envTemp = Join-Path $root ('.env.deploy.' + [Guid]::NewGuid().ToString('N'))
        $lines = if (Test-Path -LiteralPath $envFile) { [IO.File]::ReadAllLines($envFile) } else { @() }
        $result = New-Object 'System.Collections.Generic.List[string]'
        $written = $false
        foreach ($line in $lines) {
            if ($line -cmatch '^\s*(?:export\s+)?MONITOR_PORT\s*=') {
                if (-not $written) { $result.Add("MONITOR_PORT=$Port"); $written = $true }
            } else { $result.Add($line) }
        }
        if (-not $written) { $result.Add("MONITOR_PORT=$Port") }
        if (Test-Path -LiteralPath $envFile) { Copy-Item -LiteralPath $envFile -Destination $envTemp }
        [IO.File]::WriteAllLines($envTemp, $result.ToArray(), (New-Object Text.UTF8Encoding($false)))
        $dataDir = Join-Path $root 'data'
        if (-not (Test-Path -LiteralPath $dataDir)) { New-Item -ItemType Directory -Path $dataDir | Out-Null }
        if (-not $SkipBuild) {
            Write-Host '构建整合镜像；已有服务会继续运行到构建完成。'
            Invoke-Compose -Arguments @('build', 'monitor')
        }
        Write-Host "启动服务，端口 $Port"
        Invoke-Compose -Arguments @('up', '-d', '--no-build', 'monitor')
        $ids = @(Invoke-Compose -Arguments @('ps', '-q', 'monitor') | Where-Object { $_ })
        if ($ids.Count -ne 1) { throw '未找到唯一的 monitor 容器。' }
        $timer = [Diagnostics.Stopwatch]::StartNew()
        $ready = $false
        while ($timer.Elapsed.TotalSeconds -lt $Timeout) {
            $running = (Invoke-Docker -Arguments @('inspect', '--format', '{{.State.Running}}', $ids[0]) | Out-String).Trim()
            if ($running -eq 'true') {
                try {
                    # Avoid proxy settings redirecting a localhost readiness request.
                    $request = [Net.HttpWebRequest]::Create("http://127.0.0.1:$Port/")
                    $request.Proxy = $null
                    $request.Timeout = 5000
                    $request.ReadWriteTimeout = 5000
                    $response = $request.GetResponse()
                    try { $ready = ([int]$response.StatusCode -ge 200 -and [int]$response.StatusCode -lt 400) }
                    finally { $response.Close() }
                } catch { $ready = $false }
            }
            if ($ready) { break }
            Start-Sleep -Seconds 2
        }
        if (-not $ready) {
            try { Invoke-Compose -Arguments @('logs', '--tail', '60', 'monitor') } catch { }
            throw "服务在 $Timeout 秒内未就绪，未保存新的端口配置。请修复后使用相同参数重试。"
        }
        if (Test-Path -LiteralPath $envFile) {
            [IO.File]::Replace($envTemp, $envFile, [NullString]::Value)
        } else {
            [IO.File]::Move($envTemp, $envFile)
        }
        $envTemp = $null
        Write-Host "部署完成：http://127.0.0.1:$Port（远程访问请替换为服务器 IP）"
        Write-Host "首次访问按向导创建管理员账号；数据目录: $dataDir"
        Write-Host '端口已保存到 .env；更新源码后重复运行即可。日志: docker compose logs -f monitor'
    }
} catch {
    Write-Host "部署未完成: $($_.Exception.Message)" -ForegroundColor Red
    $exitCode = 1
} finally {
    [Environment]::SetEnvironmentVariable('MONITOR_PORT', $previousPort, 'Process')
    if ($envTemp -and (Test-Path -LiteralPath $envTemp)) { Remove-Item -LiteralPath $envTemp -Force }
    if ($lockOwned) { try { [IO.Directory]::Delete($lockDir, $false) } catch { } }
}
exit $exitCode
