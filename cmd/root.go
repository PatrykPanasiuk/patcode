package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"patcode/agent"
	"patcode/config"
	"patcode/llm"
	"patcode/memory"
	"patcode/session"
	"patcode/tools"
	"patcode/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "patcode [project]",
	Short: "PatCode - AI coding agent for your terminal",
	Long: `PatCode is an open-source AI coding agent that runs in your terminal.
It connects to LLM providers (OpenAI, Anthropic, Ollama, Local) and helps you write, debug, and refactor code.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
		}
		resolved, err := config.ResolveProjectDir(projectDir)
		if err != nil {
			return err
		}
		return tui.Run(resolved)
	},
}

var runCmd = &cobra.Command{
	Use:   "run [message...]",
	Short: "Run in non-interactive headless mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir, _ := cmd.Flags().GetString("project")
		mode, _ := cmd.Flags().GetString("mode")
		if len(args) == 0 {
			return fmt.Errorf("message required")
		}
		message := strings.Join(args, " ")
		return runHeadless(projectDir, mode, message)
	},
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sessions (list, delete)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Session management - not yet implemented")
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Config management - not yet implemented")
		return nil
	},
}

var trainCmd = &cobra.Command{
	Use:   "train",
	Short: "Export Codex/OpenCode sessions into memory corpus and ingest them",
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace, _ := cmd.Flags().GetString("workspace")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		codexRoot, _ := cmd.Flags().GetString("codex-root")
		opencodeRoot, _ := cmd.Flags().GetString("opencode-root")
		codexHistory, _ := cmd.Flags().GetString("codex-history")
		opencodePrompt, _ := cmd.Flags().GetString("opencode-prompt-history")
		ragRoot, _ := cmd.Flags().GetString("rag-root")
		skipIngest, _ := cmd.Flags().GetBool("skip-ingest")
		skipExport, _ := cmd.Flags().GetBool("skip-export")
		redact, _ := cmd.Flags().GetBool("redact")

		opts := memory.DefaultOptions(workspace)
		if outputDir != "" {
			opts.OutputDir = outputDir
		}
		if codexRoot != "" {
			opts.CodexRoot = codexRoot
		}
		if opencodeRoot != "" {
			opts.OpenCodeRoot = opencodeRoot
		}
		if codexHistory != "" {
			opts.CodexHistoryPath = codexHistory
		}
		if opencodePrompt != "" {
			opts.OpenCodePromptPath = opencodePrompt
		}
		if ragRoot != "" {
			opts.RagRoot = ragRoot
		}
		opts.RedactSecrets = redact

		if !skipExport {
			stats, err := memory.Export(opts)
			if err != nil {
				return err
			}
			fmt.Printf("exported %d session docs, %d prompt docs, %d dataset lines into %s\n",
				stats.SessionDocs, stats.PromptDocs, stats.DatasetLines, opts.OutputDir)
		}

		if skipIngest {
			return nil
		}
		corpusDir := filepath.Join(opts.OutputDir, "corpus")
		if err := memory.Ingest(context.Background(), opts.RagRoot, corpusDir); err != nil {
			return err
		}
		fmt.Printf("ingested memory corpus from %s\n", corpusDir)
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("patcode v0.1.0")
	},
}

func Execute() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(trainCmd)
	rootCmd.AddCommand(versionCmd)
	runCmd.Flags().StringP("project", "p", ".", "project directory")
	runCmd.Flags().String("mode", "ask", "mode: ask|plan|build|shell")
	trainCmd.Flags().String("workspace", ".", "workspace root")
	trainCmd.Flags().String("output-dir", "", "output directory for memory corpus")
	trainCmd.Flags().String("codex-root", "", "Codex sessions root")
	trainCmd.Flags().String("opencode-root", "", "OpenCode tool-output root")
	trainCmd.Flags().String("codex-history", "", "Codex history JSONL")
	trainCmd.Flags().String("opencode-prompt-history", "", "OpenCode prompt history JSONL")
	trainCmd.Flags().String("rag-root", "", "rag project root")
	trainCmd.Flags().Bool("skip-export", false, "skip export step")
	trainCmd.Flags().Bool("skip-ingest", false, "skip ingest step")
	trainCmd.Flags().Bool("redact", true, "redact secrets from exported corpus")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runHeadless(projectDir, mode, message string) error {
	absDir, err := config.ResolveProjectDir(projectDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(filepath.Join(absDir, "patcode.yaml"))
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	saveDir := cfg.SessionDir
	if saveDir == "" {
		saveDir = filepath.Join(home, ".patcode", "sessions")
	}
	sess := session.New(absDir, saveDir)
	switch strings.ToLower(mode) {
	case "plan":
		sess.SetMode(session.ModePlan)
	case "build":
		sess.SetMode(session.ModeBuild)
	case "shell":
		sess.SetMode(session.ModeShell)
	default:
		sess.SetMode(session.ModeAsk)
	}

	provider, err := llm.NewProvider(string(cfg.Provider), cfg.APIKey, cfg.ModelPath)
	if err != nil {
		return err
	}
	registry := tools.DefaultRegistry(absDir)

	if strings.ToLower(mode) == "shell" {
		cmd := exec.Command("bash", "-lc", message)
		cmd.Dir = absDir
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			fmt.Print(string(out))
		}
		return err
	}

	ag := agent.New(provider, registry, sess, cfg.Model)
	events := make(chan agent.AgentEvent)
	go ag.Process(context.Background(), message, events)

	for ev := range events {
		switch ev.Type {
		case agent.EventChunk:
			fmt.Print(ev.Content)
		case agent.EventError:
			return fmt.Errorf(ev.Error)
		}
	}
	fmt.Println()
	return nil
}
