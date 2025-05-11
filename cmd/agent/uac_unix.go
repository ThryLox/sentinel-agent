//go:build !windows

package main

import (
	"fmt"
	"os"
)

func isAdmin() bool {
	// Simple check for root on Unix-like systems
	return os.Geteuid() == 0
}

func runMeElevated() {
	fmt.Println("This agent requires root privileges. Please run with sudo.")
	os.Exit(1)
}
