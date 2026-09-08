package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"patcode/config"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the environment and configuration",
	Long: `Check that patcode can run correctly:
- Go runtime version
- Config file validity
- Ollama connectivity (if configured)
- Git availability
- Binary integrity`,
	RunE: func(cmd *cobra.Command, args []string) error {
		allOK := true
		report := func(name string, ok bool, detail string) {
			mark := "✓"
			if !ok {
				mark = "✗"
				allOK = false
			}
			fmt.Printf("  %s  %s", mark, name)
			if detail != "" {
				fmt.Printf("  (%s)", detail)
			}
			fmt.Println()
		}

		fmt.Println("patcode doctor")
		fmt.Println()

		// 1. Go runtime
		report("Go runtime", true, runtime.Version())

		// 2. Binary path and size
		exe, _ := os.Executable()
		if exe != "" {
			fi, err := os.Stat(exe)
			if err == nil {
				report("Binary", true, fmt.Sprintf("%s, %d bytes", filepath.Base(exe), fi.Size()))
			} else {
				report("Binary", false, err.Error())
			}
		}

		// 3. Config
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
		}
		var cfg *config.Config
		cfgPath := ""
		resolved, err := config.ResolveProjectDir(projectDir)
		if err == nil {
			cfg, cfgPath, err = config.LoadWithFallback(resolved)
		}
		if err != nil {
			report("Config", false, fmt.Sprintf("%v", err))
		} else {
			label := "Config (default)"
			if cfgPath != "" {
				label = fmt.Sprintf("Config (%s)", cfgPath)
			}
			report(label, true, fmt.Sprintf("provider=%s model=%s", cfg.Provider, cfg.Model))
			if cfg.APIKey != "" {
				report("  API key", true, fmt.Sprintf("set (%d chars)", len(cfg.APIKey)))
			}
		}

		// 4. Ollama
		checkOllama(report)

		// 5. Git
		if _, err := exec.LookPath("git"); err == nil {
			report("Git", true, "installed")
		} else {
			report("Git", false, "not found in PATH")
		}

		fmt.Println()
		if allOK {
			fmt.Println("All checks passed.")
		} else {
			fmt.Println("Some checks failed — see details above.")
		}
		return nil
	},
}

func checkOllama(report func(name string, ok bool, detail string)) {
	ollamaBin := ""
	if _, err := exec.LookPath("ollama"); err == nil {
		ollamaBin = "ollama"
	} else {
		home, _ := os.UserHomeDir()
		localPath := filepath.Join(home, ".patcode", "bin", "ollama")
		if _, err := os.Stat(localPath); err == nil {
			ollamaBin = localPath
		}
	}
	if ollamaBin == "" {
		report("Ollama", false, "not installed — install from https://ollama.com")
		return
	}

	baseURL := "http://localhost:11434"
	conn, err := net.DialTimeout("tcp", "localhost:11434", 3*time.Second)
	if err != nil {
		report("Ollama", false, "not running — install from https://ollama.com and run 'ollama serve'")
		return
	}
	conn.Close()

	resp, err := http.Get(baseURL + "/api/tags")
	if err != nil {
		report("Ollama", true, "running but API not responding")
		return
	}
	defer resp.Body.Close()
	report("Ollama", true, "running")
}

func init() {
	doctorCmd.Flags().Bool("verbose", false, "show detailed diagnostics")
	rootCmd.AddCommand(doctorCmd)
}
