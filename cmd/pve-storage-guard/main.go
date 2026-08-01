package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version)
		return
	}
	usage()
	if len(args) > 0 {
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(flag.CommandLine.Output(), "pve-storage-guard — Adaptive I/O protection for Proxmox VE hosts")
	fmt.Fprintln(flag.CommandLine.Output(), "")
	fmt.Fprintln(flag.CommandLine.Output(), "Status: pre-release; observer/shadow only")
	fmt.Fprintln(flag.CommandLine.Output(), "")
	fmt.Fprintln(flag.CommandLine.Output(), "Usage:")
	fmt.Fprintln(flag.CommandLine.Output(), "  pve-storage-guard version")
}
