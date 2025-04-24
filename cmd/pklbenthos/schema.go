package main

import (
	"fmt"
	"strings"

	"github.com/pauloborges/pklbenthos/internal/distribution"
	"github.com/spf13/cobra"
)

func schemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "schema REF",
		GroupID: basicCommands,
		Short:   "Print the configuration schema that a container image holds.",
		Long: `Print the configuration schema of a released version.

The command runs a container image with Docker, and writes the schema that the
image gives it:

    docker run --rm IMAGE list --format json-full

REF takes one of two forms. A reference of the form "DISTRIBUTION:VERSION"
names a known distribution. Every other reference goes to Docker as it is,
which reaches a build of your own. The generate command reads the same two
forms in --from-image.

Docker must be on the path. Send the schema to a file, or to the generate
command, which reads standard input.

Distributions:

    ` + strings.Join(distribution.Describe(), "\n    ") + `

Examples:

    pklbenthos schema redpanda-connect:4.103.1 > schema.json
    pklbenthos schema ghcr.io/example/my-connect:1.0.0 > schema.json
    pklbenthos schema redpanda-connect:4.103.1 | pklbenthos generate -o out/`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]

			// The name of a distribution on its own carries no version.
			// Docker would look for an image of that name, and report that it
			// found none. Say what is missing instead.
			if _, ok := distribution.Lookup(ref); ok {
				return fmt.Errorf("%q has no version: give %q", ref, ref+":VERSION")
			}

			image, _ := distribution.Resolve(ref)

			// Docker writes the progress of a pull to standard error, so the
			// schema on standard output stays clean.
			data, err := distribution.Fetch(cmd.Context(), image, cmd.ErrOrStderr())
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			if _, err := out.Write(data); err != nil {
				return fmt.Errorf("write schema: %w", err)
			}

			if _, err := fmt.Fprintln(out); err != nil {
				return fmt.Errorf("write schema: %w", err)
			}

			return nil
		},
	}
}
