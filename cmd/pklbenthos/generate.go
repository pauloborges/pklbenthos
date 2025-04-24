package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pauloborges/pklbenthos/internal/distribution"
	"github.com/pauloborges/pklbenthos/internal/osutil"
	"github.com/spf13/cobra"
)

func generateCmd() *cobra.Command {
	var (
		outputDir    string
		fromImage    string
		name         string
		release      string
		baseURL      string
		sourceCode   string
		issueTracker string
		author       string
		license      string
		modulePrefix string
		productName  string
		docsBaseURL  string
	)

	cmd := &cobra.Command{
		Use:     "generate [FILE]",
		GroupID: basicCommands,
		Short:   "Turn a configuration schema into a Pkl library.",
		Long: `Generate a Pkl library from a Benthos configuration schema.

The library is ready to publish. It holds one module for each component, and
the three files that make a directory of modules a Pkl package: PklProject,
LICENSE, and doc-package-info.pkl for Pkldoc.

The command reads a schema in the V0 JSON encoding. Every Benthos-compatible
distribution exposes its schema through the list command:

    redpanda-connect list --format json-full
    benthos list --format json-full
    my-connect list --format json-full

Give the schema in one of three ways. Name a schema file in FILE, or pipe the
schema into standard input, or set --from-image to read a container image with
Docker. A FILE of "-" also reads standard input.

Examples:

    redpanda-connect list --format json-full | pklbenthos generate -o out/
    pklbenthos generate schema.json -o out/
    pklbenthos generate --from-image redpanda-connect:4.103.1 -o out/
    pklbenthos generate schema.json -o out/ --name my-connect --release 1.4.0`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && fromImage != "" {
				return errors.New("give a FILE or --from-image, but not both")
			}

			opts := libraryOptions{
				Name:         name,
				Release:      release,
				BaseURL:      baseURL,
				SourceCode:   sourceCode,
				IssueTracker: issueTracker,
				Author:       author,
				License:      license,
				ModulePrefix: modulePrefix,
				ProductName:  productName,
				DocsBaseURL:  docsBaseURL,
			}

			var data []byte
			var err error

			if fromImage != "" {
				image, dist := distribution.Resolve(fromImage)

				// A known distribution says which build the schema comes
				// from, so it fills every field that the caller left alone.
				if dist != nil {
					_, tag, _ := strings.Cut(fromImage, ":")

					fill(cmd, &opts, fromDistribution(dist, tag))
				}

				data, err = distribution.Fetch(cmd.Context(), image, cmd.ErrOrStderr())
			} else {
				data, err = readSchema(cmd, args)
			}

			if err != nil {
				return err
			}

			// A schema carries the version of the build that wrote it, which
			// stands in for a release that the caller did not name.
			if opts.Release == "" {
				opts.Release = releaseOf(data)
			}

			if opts.Name == "" {
				return errors.New("no --name: say what to call the package, or use --from-image with a known distribution")
			}

			if !distribution.IsVersion(opts.Release) {
				return fmt.Errorf("release %q is not a version of three numbers: give --release", opts.Release)
			}

			fsys, err := buildLibrary(data, opts)
			if err != nil {
				return fmt.Errorf("generate Pkl library: %w", err)
			}

			if err := osutil.ReplaceDirContents(outputDir, fsys); err != nil {
				return fmt.Errorf("write Pkl modules to file system: %w", err)
			}

			return nil
		},
	}

	flags := cmd.Flags()

	flags.StringVarP(
		&outputDir,
		"output",
		"o",
		"",
		"Directory that receives the Pkl modules. The command removes the contents of the directory first.",
	)

	flags.StringVar(
		&fromImage,
		"from-image",
		"",
		"Read the schema from a container image, named by a REF. A REF of the form \"DISTRIBUTION:VERSION\" names a "+
			"known distribution, and every other REF goes to Docker as it is. A known DISTRIBUTION also names the "+
			"library. Distributions: "+strings.Join(distribution.SortedNames(), ", ")+".",
	)

	flags.StringVar(&name, "name", "",
		"Name of the build, such as \"my-connect\". It goes in front of the release in the name of the package. "+
			"A known DISTRIBUTION in --from-image gives its own.")
	flags.StringVar(&release, "release", "",
		"Version of the build, such as \"1.0.0\". The default is the version that the schema itself carries.")
	flags.StringVar(&baseURL, "base-url", "",
		"Directory that holds the published artifacts, such as "+
			"\"https://github.com/OWNER/REPO/releases/download\". A package writes its metadata to "+
			"BASE/TAG/TAG and its modules to BASE/TAG/TAG.zip, which is the layout that a GitHub release gives.")
	flags.StringVar(&sourceCode, "source-code", "", "URL of the source of the project.")
	flags.StringVar(&issueTracker, "issue-tracker", "", "URL of the place to report a problem.")
	flags.StringVar(&author, "author", "",
		"Maintainer of the package, as an RFC5322 mailbox, such as \"Ada Lovelace <ada@example.com>\". Required.")
	flags.StringVar(&license, "license", "",
		"SPDX identifier of the license of the build, such as \"Apache-2.0\". "+
			"A known DISTRIBUTION in --from-image gives its own.")

	flags.StringVar(
		&modulePrefix,
		"module-prefix",
		"",
		`Prefix for the name of each generated module, such as "com.example.connect". An empty prefix leaves the names bare. `+
			`A known DISTRIBUTION in --from-image gives its own default.`,
	)

	flags.StringVar(
		&productName,
		"product-name",
		"",
		`Name of the build that reads the configuration. The generated documentation speaks of the build by this name. `+
			`An empty name gives "Benthos". A known DISTRIBUTION in --from-image gives its own default.`,
	)

	flags.StringVar(
		&docsBaseURL,
		"docs-base-url",
		"",
		`Root of the documentation site of the build, such as "https://docs.redpanda.com/redpanda-connect". `+
			`The generator turns each cross reference of the schema into a link under this root. `+
			`An empty URL leaves a cross reference as the schema wrote it. `+
			`A known DISTRIBUTION in --from-image gives its own default.`,
	)

	for _, name := range []string{"output", "author", "base-url"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}

	return cmd
}

// readSchema reads a schema in the V0 JSON encoding. It reads the file that
// FILE names. It reads standard input when the caller gives no FILE, or a
// FILE of "-".
func readSchema(cmd *cobra.Command, args []string) ([]byte, error) {
	if len(args) == 1 && args[0] != "-" {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return nil, fmt.Errorf("read schema file: %w", err)
		}

		return data, nil
	}

	stdin := cmd.InOrStdin()

	// A terminal on standard input means that no one piped a schema in. Say so
	// instead of waiting for input that does not come.
	if file, ok := stdin.(*os.File); ok {
		info, err := file.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return nil, errors.New(
				"no FILE, and standard input is a terminal: name a file, pipe a schema in, or set --from-image")
		}
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read schema from standard input: %w", err)
	}

	if len(data) == 0 {
		return nil, errors.New("read schema from standard input: no data")
	}

	return data, nil
}
