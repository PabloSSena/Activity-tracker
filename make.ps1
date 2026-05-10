param([string]$Task = 'build')

$BinaryName = 'activitytracker'
$BuildDir   = 'bin'
$CmdDir     = 'cmd/activitytracker'
$Binary     = "$BuildDir/$BinaryName.exe"

$TestPkgs = @(
    './internal/autostart/...'
    './internal/config/...'
    './internal/monitor/classifier/...'
    './internal/monitor/collector/...'
    './internal/report/...'
    './internal/storage/...'
)

function Invoke-Build {
    go build -o $Binary ./$CmdDir
}

switch ($Task) {
    'build' {
        Invoke-Build
    }
    'test' {
        go test $TestPkgs
    }
    'lint' {
        go vet ./...
    }
    'run' {
        Invoke-Build
        if ($LASTEXITCODE -eq 0) { & "./$Binary" }
    }
    'clean' {
        if (Test-Path $BuildDir) { Remove-Item -Recurse -Force $BuildDir }
    }
    default {
        Write-Host "Tarefas disponíveis: build, test, lint, run, clean"
        exit 1
    }
}
