param(
    [Parameter(Mandatory = $true)][string]$Installer,
    [Parameter(Mandatory = $true)][string]$NumericVersion
)

$ErrorActionPreference = "Stop"
$Installer = (Resolve-Path $Installer).Path

$work = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { $env:TEMP }
$dir = Join-Path $work 'PixGoInstallTest'
$log = Join-Path $work 'pixgo-install.log'
$extensions = @('.bmp', '.cur', '.dib', '.gif', '.ico', '.jfif', '.jpe', '.jpeg', '.jpg', '.png', '.svg', '.svgz', '.tif', '.tiff', '.webp')
$progId = 'PixGo.Image'
$failures = New-Object System.Collections.Generic.List[string]

function Check($ok, $message) {
    if ($ok) { Write-Host "ok    $message" } else { Write-Host "FAIL  $message"; $failures.Add($message) }
}

function Install($tasks) {
    $args = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/CURRENTUSER', "/TASKS=$tasks", "/DIR=$dir", "/LOG=$log")
    $p = Start-Process -FilePath $Installer -ArgumentList $args -Wait -PassThru
    Check ($p.ExitCode -eq 0) "installer exit code is $($p.ExitCode) (tasks '$tasks')"
}

function Uninstall {
    $unins = Get-ChildItem -Path $dir -Filter 'unins*.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
    Check ($null -ne $unins) 'uninstaller was installed'
    if ($unins) {
        $p = Start-Process -FilePath $unins.FullName -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') -Wait -PassThru
        Check ($p.ExitCode -eq 0) "uninstaller exit code is $($p.ExitCode)"
    }
    for ($i = 0; $i -lt 20 -and (Test-Path (Join-Path $dir 'pixgo.exe')); $i++) { Start-Sleep -Milliseconds 500 }
}

function HasProgId($ext) {
    $key = "HKCU:\Software\Classes\$ext\OpenWithProgids"
    if (-not (Test-Path $key)) { return $false }
    return $null -ne (Get-Item $key).GetValue($progId, $null) -or ((Get-Item $key).GetValueNames() -contains $progId)
}

Write-Host "== install with the association task"
Install 'assoc'
Check (Test-Path (Join-Path $dir 'pixgo.exe')) 'pixgo.exe installed'
Check (Test-Path (Join-Path $dir 'pixgo.ico')) 'pixgo.ico installed'
Check (Test-Path (Join-Path $dir 'LICENSE')) 'LICENSE installed'
Check (Test-Path (Join-Path $dir 'third_party')) 'third_party licenses installed'
$info = (Get-Item (Join-Path $dir 'pixgo.exe')).VersionInfo
Check ($info.ProductVersion -eq $NumericVersion) "product version is $($info.ProductVersion), want $NumericVersion"
Check ($info.ProductName -eq 'PixGo') 'product name is PixGo'

foreach ($ext in $extensions) {
    Check (HasProgId $ext) "$ext lists $progId in OpenWithProgids"
}
$cmd = (Get-ItemProperty "HKCU:\Software\Classes\$progId\shell\open\command" -ErrorAction SilentlyContinue).'(default)'
Check ($cmd -like '*pixgo.exe*"%1"*') "open command is $cmd"
$caps = Get-ItemProperty 'HKCU:\Software\PixGo\Capabilities\FileAssociations' -ErrorAction SilentlyContinue
foreach ($ext in $extensions) {
    Check ($caps -and $caps.$ext -eq $progId) "capabilities map $ext to $progId"
}
$reg = Get-ItemProperty 'HKCU:\Software\RegisteredApplications' -ErrorAction SilentlyContinue
Check ($reg -and $reg.PixGo -eq 'Software\PixGo\Capabilities') 'registered application points to the capabilities'

Write-Host "== uninstall removes the association"
Uninstall
Check (-not (Test-Path (Join-Path $dir 'pixgo.exe'))) 'pixgo.exe removed'
foreach ($ext in $extensions) {
    Check (-not (HasProgId $ext)) "$ext no longer lists $progId"
}
Check (-not (Test-Path "HKCU:\Software\Classes\$progId")) 'ProgID key removed'
Check (-not (Test-Path 'HKCU:\Software\PixGo')) 'capabilities key removed'
$reg = Get-ItemProperty 'HKCU:\Software\RegisteredApplications' -ErrorAction SilentlyContinue
Check (-not ($reg -and $null -ne $reg.PixGo)) 'registered application removed'

Write-Host "== install without the association task"
Install ''
Check (Test-Path (Join-Path $dir 'pixgo.exe')) 'pixgo.exe installed without associations'
Check (-not (HasProgId '.jpg')) '.jpg is not associated when the task is off'
Check (-not (Test-Path "HKCU:\Software\Classes\$progId")) 'ProgID is not created when the task is off'
Uninstall

if ($failures.Count -gt 0) {
    Write-Host ""
    Write-Host "$($failures.Count) check(s) failed:"
    $failures | ForEach-Object { Write-Host "  - $_" }
    if (Test-Path $log) { Get-Content $log -Tail 40 }
    exit 1
}
Write-Host "installer verified"
