// Ottie - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Ottie contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jiayaoqijia/ottie/cmd/ottie/internal"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/agent"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/auth"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/cron"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/gateway"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/migrate"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/model"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/onboard"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/skills"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/status"
	"github.com/jiayaoqijia/ottie/cmd/ottie/internal/version"
	"github.com/jiayaoqijia/ottie/pkg/config"
)

func NewOttieCommand() *cobra.Command {
	short := fmt.Sprintf("%s ottie - Self-Evolving AI Intern v%s\n\n", internal.Logo, config.GetVersion())

	cmd := &cobra.Command{
		Use:     "ottie",
		Short:   short,
		Example: "ottie version",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		model.NewModelCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

const (
	colorBlue = "\033[1;38;2;62;93;185m"
	banner    = "\r\n" +
		colorBlue + " ██████╗ ████████╗████████╗██╗███████╗\n" +
		colorBlue + "██╔═══██╗╚══██╔══╝╚══██╔══╝██║██╔════╝\n" +
		colorBlue + "██║   ██║   ██║      ██║   ██║█████╗\n" +
		colorBlue + "██║   ██║   ██║      ██║   ██║██╔══╝\n" +
		colorBlue + "╚██████╔╝   ██║      ██║   ██║███████╗\n" +
		colorBlue + " ╚═════╝    ╚═╝      ╚═╝   ╚═╝╚══════╝\n " +
		"\033[0m\r\n"
)

func main() {
	fmt.Printf("%s", banner)
	cmd := NewOttieCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
