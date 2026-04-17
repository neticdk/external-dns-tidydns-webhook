package main

import (
	"os"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd"
)

var version = "HEAD"

func main() {
	os.Exit(cmd.Execute(version))
}
