set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

# build debug executable and run
run: build
    .\mpvrc.exe

# build debug executable
build:
    go build

# build release executable
build-release: _embed-resources build

_embed-resources:
    go-winres make
