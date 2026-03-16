//go:build windows
// +build windows

package ui

import "os/exec"

func isGatewayProcessRunning() bool {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq ottie.exe")
	return cmd.Run() == nil
}

func stopGatewayProcess() error {
	cmd := exec.Command("taskkill", "/F", "/IM", "ottie.exe")
	return cmd.Run()
}
