// Command pve-storage-guard is the product CLI and service entrypoint.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

var version = "dev"

func main() {
	flag.Usage = func() { _ = writeUsage(flag.CommandLine.Output()) }
	flag.Parse()
	args := flag.Args()
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version)
		return
	}
	if err := writeUsage(flag.CommandLine.Output()); err != nil {
		os.Exit(1)
	}
	if len(args) > 0 {
		os.Exit(2)
	}
}

func writeUsage(writer io.Writer) error {
	_, err := fmt.Fprint(writer, `pve-storage-guard — Adaptive I/O protection for Proxmox VE hosts

Status: pre-release; observer/shadow only

Usage:
  pve-storage-guard version
`)
	return err
}
