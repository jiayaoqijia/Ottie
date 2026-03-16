package skills

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/jiayaoqijia/ottie/cmd/ottie/internal"
	"github.com/jiayaoqijia/ottie/pkg/skills"
)

func newSyncIndexCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-index",
		Short: "Sync the local skill index from ClawHub for instant offline search",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := internal.LoadConfig()
			if err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}

			if !cfg.Tools.Skills.Registries.ClawHub.Enabled {
				return fmt.Errorf("ClawHub registry is not enabled in config")
			}

			clawhubReg := skills.NewClawHubRegistry(skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub))

			localReg := skills.NewLocalIndexRegistry(skills.LocalIndexConfig{
				Enabled:   true,
				IndexPath: cfg.Tools.Skills.Registries.LocalIndex.IndexPath,
				Fallback:  clawhubReg,
			})

			fmt.Println("Syncing skill index from ClawHub...")

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			count, err := localReg.Sync(ctx, clawhubReg)
			if err != nil {
				return fmt.Errorf("sync failed: %w", err)
			}

			fmt.Printf("Synced %d skills to local index.\n", count)
			return nil
		},
	}
}
