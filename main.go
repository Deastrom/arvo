package main

import (
	"github.com/Deastrom/arvo/cmd"
	"github.com/Deastrom/arvo/internal/mcp"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	mcp.Version = version
	cmd.SetVersion(version)
	cmd.Execute()
}
