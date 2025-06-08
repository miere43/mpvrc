param(
    [Parameter(Position=0)]
    [ValidateSet("build", "run", "build-release", "test", "dist", "dev")]
    [string]$Task = "run"
)

function Error-Check {
    param(
        [string]$Message
    )
    if ($LASTEXITCODE -ne 0) {
        Write-Host $Message -ForegroundColor Red
        exit $LASTEXITCODE
    }
}

function Build {
    go build ./cmd/mpvrc
    Error-Check "build failed."
}

function Run {
    Build
    .\mpvrc.exe
}

function Test {
    go test ./...
    Error-Check "go test failed."
}

function Build-Release {
    go build ./cmd/build
    Error-Check "build helper executable failed."

    .\build.exe
    Error-Check "build helper failed."

    go build -tags index_embed -ldflags "-H=windowsgui -s -w" -trimpath ./cmd/mpvrc
    Error-Check "build release executable failed."
}

function Build-Release-Front {
    npm run build --prefix=.\front
    Error-Check "build front failed."
}

function Dist {
    Build-Release-Front
    Build-Release
}

function Dev {
    npm run dev --prefix=.\front
    Error-Check "build dev front failed."
}

switch ($Task) {
    "build" { Build }
    "run" { Run }
    "build-release" { Build-Release }
    "test" { Test }
    "dist" { Dist }
    "dev" { Dev }
}
