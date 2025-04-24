package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

const (
	basicCommands    = "basic"
	advancedCommands = "advanced"
)

func main() {
	var verboseParam bool

	// Keep the commands in the order that they go in below, so that each group
	// reads from the first step to the last. The order of the alphabet would
	// put `package` in front of `sync`, which runs the other way round.
	cobra.EnableCommandSorting = false

	logLevel := new(slog.LevelVar)
	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(logHandler))

	rootCmd := cobra.Command{
		Use:   "pklbenthos",
		Short: "Manage Pkl libraries for Benthos distributions.",
		Long: `Manage Pkl libraries for Benthos distributions.

A Benthos pipeline is described using a YAML file. pklbenthos allows
developers to author Benthos pipelines using Pkl, a programmable, scalable,
and safe configuration language.

A library holds the components of one distribution at one release, and comes
from the configuration schema that the release writes for itself. It is ready
to publish: beside the modules it carries a PklProject, a LICENSE, and the
doc-package-info.pkl that Pkldoc reads.

Every command that writes a library needs to know who maintains it and where
it will live, so --author and --base-url are required. The base URL is the
directory that holds the published artifacts, and a package writes its
metadata to BASE/TAG/TAG and its modules to BASE/TAG/TAG.zip.

BUILDING ONE LIBRARY

Build from a released version, from a build of your own, or from a schema that
you keep on disk. A known distribution brings its own naming, and a build of
your own takes --name:

  pklbenthos generate --from-image redpanda-connect:4.104.0 -o out/ \
    --author "Ada Lovelace <ada@example.com>" \
    --base-url https://github.com/me/pkl/releases/download

  pklbenthos generate schema.json -o out/ --name my-connect \
    --author "Ada Lovelace <ada@example.com>" \
    --base-url https://github.com/me/pkl/releases/download

The release comes from the schema itself, and --release says otherwise.

Read the schema of a Docker image, to keep as a file or to send through a
pipe. Any image that answers ` + "`list --format json-full`" + ` will do:

  pklbenthos schema redpanda-connect:4.104.0 > schema.json

MANY RELEASES AT ONCE

Sync lists the releases of a distribution, reads the schema of each one that
is not on disk yet, and writes a library for every one of them. It keeps the
newest releases and no more, and takes the schema and the library of every
release that falls outside that window:

  pklbenthos sync redpanda-connect --limit 10 \
    --author "Ada Lovelace <ada@example.com>" \
    --base-url https://github.com/me/pkl/releases/download

A release that leaves the window keeps its published package. The tree holds
what this repository maintains, and the releases hold what it ever published.

For a build of your own, give the repository that holds its images, and pick a
name to file it under:

  pklbenthos sync my-connect --from-repository ghcr.io/me/my-connect \
    --author "Ada Lovelace <ada@example.com>" \
    --base-url https://github.com/me/pkl/releases/download

Each library that sync writes is a Pkl package already, so publishing it is
` + "`pkl project package`" + ` on the directory.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if verboseParam {
				logLevel.Set(slog.LevelDebug)
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().BoolVarP(
		&verboseParam,
		"verbose",
		"v",
		false,
		"Verbose output.",
	)

	rootCmd.AddGroup(
		&cobra.Group{ID: basicCommands, Title: "Basic commands:"},
		&cobra.Group{ID: advancedCommands, Title: "Advanced commands:"},
	)

	rootCmd.AddCommand(generateCmd())
	rootCmd.AddCommand(schemaCmd())
	rootCmd.AddCommand(syncCmd())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		logError(err)
		os.Exit(1)
	}
}

func logError(err error) {
	var messages []string

	for err != nil {
		messages = append(messages, strings.TrimSpace(err.Error()))
		err = errors.Unwrap(err)
	}

	slices.Reverse(messages)
	var errs []string

	for idx, message := range messages {
		if idx == 0 {
			errs = append(errs, message)
			continue
		}

		message = strings.TrimSuffix(message, messages[idx-1])
		message = strings.TrimSuffix(message, ": ")

		errs = append(errs, message)
	}

	slices.Reverse(errs)

	for idx, err := range errs {
		if idx > 0 {
			slog.Error("caused by: " + err)
		} else {
			slog.Error(err)
		}
	}
}
