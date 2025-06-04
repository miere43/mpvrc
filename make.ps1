param(
    [Parameter(Position=0)]
    [ValidateSet("build", "run", "build-release")]
    [string]$Task = "run"
)

function Build {
    go build .
    if ($LASTEXITCODE -ne 0) {
        Write-Host "go build failed." -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

function Run {
    Build
    .\mpvrc.exe
}

function BuildRelease {
    go build -ldflags -H=windowsgui
    if ($LASTEXITCODE -ne 0) {
        Write-Host "go build failed." -ForegroundColor Red
        exit $LASTEXITCODE
    }
    
    go-winres make
    if ($LASTEXITCODE -ne 0) {
        Write-Host "go-winres failed." -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

switch ($Task) {
    "build" { Build }
    "run" { Run }
    "build-release" { BuildRelease }
}
