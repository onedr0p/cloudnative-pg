/*
Copyright The CloudNativePG Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package waldircopy implement the waldircopy command
package waldircopy

import (
	"errors"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/context"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/log"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres"
)

// ErrInstanceNotFenced is returned when the instance is not fenced
var ErrInstanceNotFenced = errors.New("instance not fenced")

// NewCmd creates a new cobra command
func NewCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:           "waldir-copy [source] [destination]",
		SilenceErrors: true,
		Args:          cobra.ExactArgs(2),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			contextLog := log.WithName("waldir-copy")
			contextLog.Info("Starting the waldir-copy command")
			ctx := log.IntoContext(cobraCmd.Context(), contextLog)
			err := run(ctx, args[0], args[1])
			if err == nil {
				contextLog.Info("waldir-copy completed")
				return nil
			}

			switch {
			case errors.Is(err, ErrInstanceNotFenced):
				contextLog.Info("cannot move WAL files, instance is not fenced")
			default:
				contextLog.Info("command failed", "error", err)
			}

			contextLog.Debug("There was an error in the previous waldir-copy command. Waiting 100 ms before retrying.")
			time.Sleep(100 * time.Millisecond)
			return err
		},
	}

	return &cmd
}

func run(ctx context.Context, source, destination string) error {
	contextLog := log.FromContext(ctx)

	initInfo := postgres.InitInfo{
		PgWal:  destination,
		PgData: source,
	}
	contextLog.Info("Initializing the destination directory", "initInfo", initInfo)
	changed, err := initInfo.RestoreCustomWalDir(ctx)
	if err != nil {
		contextLog.Info("failed to copy wal directory", "error", err)
		return err
	}
	if !changed {
		contextLog.Info("waldir-copy: nothing to do")
	}
	return nil
}
