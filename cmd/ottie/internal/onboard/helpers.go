package onboard

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jiayaoqijia/ottie/cmd/ottie/internal"
	"github.com/jiayaoqijia/ottie/pkg/config"
	"github.com/jiayaoqijia/ottie/pkg/tools"
)

func onboard() {
	configPath := internal.GetConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	cfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	createWorkspaceTemplates(workspace)

	fmt.Printf("%s ottie is ready!\n", internal.Logo)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add your API key to", configPath)
	fmt.Println("")
	fmt.Println("     Recommended:")
	fmt.Println("     - OpenRouter: https://openrouter.ai/keys (access 100+ models)")
	fmt.Println("     - Ollama:     https://ollama.com (local, free)")
	fmt.Println("")
	fmt.Println("     See README.md for 17+ supported providers.")
	fmt.Println("")
	fmt.Println("  2. Chat: ottie agent -m \"Hello!\"")
	fmt.Println("")

	// Check for browser availability (render_html tool)
	if tools.HasBrowser() {
		fmt.Println("  ✓ Browser detected — render_html tool is available")
	} else {
		fmt.Println("")
		fmt.Println("  ⚠ No browser found — render_html tool will be disabled")
		fmt.Println("    render_html creates rich visual cards and dashboards.")
		// Only prompt interactively when stdin is a terminal
		if isTerminal() {
			fmt.Println("")
			fmt.Print("  Install Chromium now? (y/n): ")
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) == "y" {
				installBrowser()
			} else {
				fmt.Println("")
				fmt.Println("  You can install it later:")
				printBrowserInstallHint()
			}
		} else {
			fmt.Println("")
			fmt.Println("  Install a browser to enable it:")
			printBrowserInstallHint()
		}
	}
}

// installBrowser attempts to install Chromium using the system package manager.
func installBrowser() {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			fmt.Println("  Installing via Homebrew...")
			cmd = exec.Command("brew", "install", "--cask", "google-chrome")
		} else {
			fmt.Println("  Homebrew not found. Please install Chrome manually:")
			fmt.Println("    https://www.google.com/chrome/")
			return
		}
	case "linux":
		switch {
		case hasCommand("apt-get"):
			fmt.Println("  Installing chromium-browser via apt...")
			cmd = exec.Command("sudo", "apt-get", "install", "-y", "chromium-browser")
		case hasCommand("apk"):
			fmt.Println("  Installing chromium via apk...")
			cmd = exec.Command("apk", "add", "--no-cache", "chromium")
		case hasCommand("dnf"):
			fmt.Println("  Installing chromium via dnf...")
			cmd = exec.Command("sudo", "dnf", "install", "-y", "chromium")
		case hasCommand("pacman"):
			fmt.Println("  Installing chromium via pacman...")
			cmd = exec.Command("sudo", "pacman", "-S", "--noconfirm", "chromium")
		default:
			fmt.Println("  Could not detect package manager. Please install manually:")
			printBrowserInstallHint()
			return
		}
	default:
		fmt.Println("  Auto-install not supported on this OS. Please install manually:")
		printBrowserInstallHint()
		return
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("\n  ✗ Installation failed: %v\n", err)
		fmt.Println("  Please install manually:")
		printBrowserInstallHint()
		return
	}

	if tools.HasBrowser() {
		fmt.Println("\n  ✓ Browser installed — render_html tool is now available")
	} else {
		fmt.Println("\n  ✗ Browser still not detected after install")
		fmt.Println("  You may need to restart your shell or check your PATH")
	}
}

func printBrowserInstallHint() {
	switch runtime.GOOS {
	case "darwin":
		fmt.Println("    brew install --cask google-chrome")
	case "linux":
		fmt.Println("    # Debian/Ubuntu")
		fmt.Println("    sudo apt install chromium-browser")
		fmt.Println("")
		fmt.Println("    # Alpine")
		fmt.Println("    apk add chromium")
	default:
		fmt.Println("    Install Google Chrome or Chromium")
	}
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func createWorkspaceTemplates(workspace string) {
	err := copyEmbeddedToTarget(workspace)
	if err != nil {
		fmt.Printf("Error copying workspace templates: %v\n", err)
	}
}

func copyEmbeddedToTarget(targetDir string) error {
	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("Failed to create target directory: %w", err)
	}

	// Walk through all files in embed.FS
	err := fs.WalkDir(embeddedFiles, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Read embedded file
		data, err := embeddedFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("Failed to read embedded file %s: %w", path, err)
		}

		new_path, err := filepath.Rel("workspace", path)
		if err != nil {
			return fmt.Errorf("Failed to get relative path for %s: %v\n", path, err)
		}

		// Build target file path
		targetPath := filepath.Join(targetDir, new_path)

		// Ensure target file's directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("Failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}

		// Write file
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			return fmt.Errorf("Failed to write file %s: %w", targetPath, err)
		}

		return nil
	})

	return err
}
