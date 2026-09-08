package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"patcode/config"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Stwórz domyślną konfigurację PatCode",
	Long: `Tworzy domyślną konfigurację w ~/.patcode/patcode.yaml.

PatCode domyślnie używa wbudowanego providera (builtin),
który nie wymaga żadnych zewnętrznych zależności.

Aby użyć prawdziwego modelu AI, ustaw provider: openrouter
(z kluczem api_key) albo zainstaluj Ollamę (ollama.com).`,

	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetup()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup() error {
	fmt.Println("=== PatCode Setup ===")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	cfgDir := filepath.Join(home, ".patcode")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return err
	}

	cfgPath := filepath.Join(cfgDir, "patcode.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("✓ Konfiguracja już istnieje: %s\n", cfgPath)
		return nil
	}

	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}

	fmt.Printf("✓ Utworzono: %s\n", cfgPath)
	fmt.Printf("  provider: builtin (gotowy do użycia, zero zależności)\n")
	fmt.Println()
	fmt.Println("Uruchom PatCode:")
	fmt.Println("  patcode .                    # interaktywny TUI")
	fmt.Println("  patcode run --mode shell ...  # shell")
	fmt.Println()
	fmt.Println("Aby użyć prawdziwego modelu AI:")
	fmt.Println("  1. Użyj OpenRouter: ustaw w ~/.patcode/patcode.yaml")
	fmt.Println("       provider: openrouter")
	fmt.Println("       api_key: sk-or-v1-...")
	fmt.Println("       model: openrouter/auto")
	fmt.Println("       base_url: https://openrouter.ai/api/v1")
	fmt.Println("  2. Albo zainstaluj Ollamę: curl -fsSL https://ollama.com/install.sh | sh")
	return nil
}
