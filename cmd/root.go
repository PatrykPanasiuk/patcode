package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"patcode/agent"
	"patcode/config"
	"patcode/llm"
	"patcode/memory"
	"patcode/session"
	"patcode/tools"
	"patcode/tui"
	"patcode/version"
)

var rootCmd = &cobra.Command{
	Use:   "patcode [project]",
	Short: "PatCode - AI coding agent for your terminal",
	Long: `PatCode is an open-source AI coding agent that runs in your terminal.
It connects to LLM providers (OpenAI, OpenRouter, Anthropic, Ollama, Local) and helps you write, debug, and refactor code.`,
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
		cfg, _, err := config.LoadWithFallback(resolved)
		if err != nil {
			return err
		}
		return tui.Run(resolved, cfg)
	},
}

func init() {
	runCmd.Flags().StringP("project", "p", ".", "project directory")
	runCmd.Flags().String("mode", "ask", "mode: ask|inspect|plan|review|audit|patch|build|fix|refactor|scaffold|test|ci|shell")
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
	Use:   "session [list|show|restore|delete]",
	Short: "Manage sessions",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		saveDir := filepath.Join(home, ".patcode", "sessions")

		if len(args) == 0 {
			return listSessions(saveDir)
		}
		switch args[0] {
		case "list":
			return listSessions(saveDir)
		case "show":
			id := "."
			if len(args) > 1 {
				id = args[1]
			}
			return showSession(saveDir, id)
		case "delete":
			if len(args) < 2 {
				return fmt.Errorf("session id required")
			}
			return deleteSession(saveDir, args[1])
		case "restore":
			if len(args) < 2 {
				return fmt.Errorf("session id required")
			}
			return restoreSession(saveDir, args[1])
		default:
			return showSession(saveDir, args[0])
		}
	},
}

func listSessions(saveDir string) error {
	sessions, err := session.ListSessions(saveDir)
	if err != nil {
		return fmt.Errorf("reading sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	fmt.Printf("Sessions (%d):\n", len(sessions))
	for _, s := range sessions {
		project := s.Project
		if project == "" {
			project = "-"
		}
		mode := string(s.Mode)
		if mode == "" {
			mode = "ask"
		}
		fmt.Printf("  %-20s  %-6s  %3d msgs  %s\n",
			s.ID, mode, len(s.Messages), project)
	}
	return nil
}

func showSession(saveDir, id string) error {
	if id == "." {
		return listSessions(saveDir)
	}
	sess, err := session.Load(id, saveDir)
	if err != nil {
		return fmt.Errorf("loading session %s: %w", id, err)
	}
	fmt.Printf("Session: %s\n", sess.ID)
	fmt.Printf("Created: %s\n", sess.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", sess.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Mode:    %s\n", sess.Mode)
	fmt.Printf("Messages: %d\n", len(sess.Messages))
	fmt.Printf("Project: %s\n", sess.Project)
	return nil
}

func deleteSession(saveDir, id string) error {
	path := filepath.Join(saveDir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session %s not found", id)
		}
		return fmt.Errorf("deleting session: %w", err)
	}
	fmt.Printf("Deleted session %s\n", id)
	return nil
}

func restoreSession(saveDir, id string) error {
	sess, err := session.Load(id, saveDir)
	if err != nil {
		return fmt.Errorf("loading session %s: %w", id, err)
	}

	projectDir := sess.Project
	if projectDir == "" {
		projectDir = "."
	}
	absDir, err := config.ResolveProjectDir(projectDir)
	if err != nil {
		return fmt.Errorf("resolving session project: %w", err)
	}
	cfg, _, err := config.LoadWithFallback(absDir)
	if err != nil {
		return err
	}
	fmt.Printf("Restoring session %s (%d messages, project %s)\n", sess.ID, len(sess.Messages), absDir)
	return tui.RunWithSession(absDir, cfg, sess)
}

var configCmd = &cobra.Command{
	Use:   "config [show|init]",
	Short: "Manage configuration",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		if len(args) > 0 && args[0] == "init" {
			return initConfig(projectDir)
		}
		return showConfig(projectDir)
	},
}

func showConfig(projectDir string) error {
	resolved, err := config.ResolveProjectDir(projectDir)
	if err != nil {
		return fmt.Errorf("no project config found: %w", err)
	}
	cfg, cfgPath, err := config.LoadWithFallback(resolved)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfgPath == "" {
		fmt.Println("Config: <default>")
	} else {
		fmt.Printf("Config: %s\n", cfgPath)
	}
	fmt.Printf("  provider:   %s\n", cfg.Provider)
	fmt.Printf("  model:      %s\n", cfg.Model)
	fmt.Printf("  base_url:   %s\n", cfg.BaseURL)
	fmt.Printf("  model_path: %s\n", cfg.ModelPath)
	fmt.Printf("  temperature: %.1f\n", cfg.Temperature)
	fmt.Printf("  max_tokens: %d\n", cfg.MaxTokens)
	if cfg.APIKey != "" {
		fmt.Printf("  api_key:    %s...%s (%d chars)\n",
			cfg.APIKey[:4], cfg.APIKey[len(cfg.APIKey)-4:], len(cfg.APIKey))
	}
	fmt.Printf("  session_dir: %s\n", cfg.SessionDir)
	fmt.Printf("  permissions:\n")
	fmt.Printf("    auto_approve: %v\n", cfg.Permissions.AutoApprove)
	fmt.Printf("    default_deny: %v\n", cfg.Permissions.DefaultDeny)
	return nil
}

func initConfig(projectDir string) error {
	resolved, err := config.ResolveProjectDir(projectDir)
	if err != nil {
		return fmt.Errorf("cannot resolve project: %w", err)
	}
	cfgPath := filepath.Join(resolved, "patcode.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("config already exists at %s", cfgPath)
	}
	cfg := config.DefaultConfig()
	cfg.Provider = config.ProviderOllama
	cfg.Model = "llama3"
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Created default config at %s\n", cfgPath)
	return nil
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
		fmt.Printf("patcode %s\n", version.Version)
	},
}

func Execute() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(trainCmd)
	rootCmd.AddCommand(versionCmd)

	PrintBrandArt()
	MaybeCheckForUpdates()

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
	cfg, _, err := config.LoadWithFallback(absDir)
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	saveDir := cfg.SessionDir
	if saveDir == "" {
		saveDir = filepath.Join(home, ".patcode", "sessions")
	}
	sess := session.New(absDir, saveDir)

	parsedMode, err := session.ParseMode(strings.ToLower(mode))
	if err != nil {
		return fmt.Errorf("invalid mode: %w", err)
	}
	sess.SetMode(parsedMode)

	provider, err := llm.NewProvider(llm.ProviderConfig{
		Type:      string(cfg.Provider),
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		ModelPath: cfg.ModelPath,
		BaseURL:   cfg.BaseURL,
	})
	if err != nil {
		return err
	}
	registry := tools.DefaultRegistry(absDir)

	if parsedMode == session.ModeShell {
		cmd := exec.Command("bash", "-lc", message)
		cmd.Dir = absDir
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			fmt.Print(string(out))
		}
		return err
	}

	ag := agent.New(provider, registry, sess, cfg.Model, cfg.MaxTurns)
	events := make(chan agent.AgentEvent)
	go ag.Process(context.Background(), message, events)

	for ev := range events {
		switch ev.Type {
		case agent.EventChunk:
			fmt.Print(ev.Content)
		case agent.EventError:
			return errors.New(ev.Error)
		}
	}
	fmt.Println()
	return nil
}
