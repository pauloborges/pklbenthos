package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pauloborges/pklbenthos/internal/distribution"
	"github.com/pauloborges/pklbenthos/internal/osutil"
	"github.com/pauloborges/pklbenthos/internal/pkllib"
	"github.com/spf13/cobra"
)

// defaultLimit is how many of the newest releases the tree holds. A release
// that falls outside the window leaves the tree, and its published package
// stays where it is.
const defaultLimit = 10

func syncCmd() *cobra.Command {
	var (
		root           string
		baseURL        string
		sourceCode     string
		issueTracker   string
		fromRepository string
		tagSuffix      string
		modulePrefix   string
		productName    string
		docsBaseURL    string
		license        string
		author         string
		since          string
		limit          int
		prune          bool
		dryRun         bool
	)

	cmd := &cobra.Command{
		Use:     "sync NAME",
		GroupID: advancedCommands,
		Short:   "Read the schema of each new release and write the libraries.",
		Long: `Harvest the schema of every release and generate the Pkl libraries.

The command lists the releases of a distribution in its container registry,
reads the schema of each one that is not on disk yet, and generates a library
from every schema.

Every release gets a library of its own, and each one is ready to publish:

    schemas/NAME/VERSION.json    the schema of each release
    pkl/NAME/VERSION/            the library of that release
    pkl/NAME/versions.json       the releases that the tree holds

--limit says how many of the newest releases the tree keeps, and the default
is 10. A release that falls outside the window loses its schema and its
library, and keeps the package that it already published.

The command writes the same tree every time it runs, from the schemas that sit
under --root. Docker must be on the path to read a schema that is missing.

NAME is a distribution that this program knows, which brings its own image and
its own naming. For a build of your own, give --from-repository with the
repository that holds the image of every release, and NAME then files the
output and names the packages.

Examples:

    pklbenthos sync redpanda-connect --limit 5 --dry-run \
      --author "Ada Lovelace <ada@example.com>" \
      --base-url https://github.com/me/pkl/releases/download

    pklbenthos sync my-connect --from-repository ghcr.io/me/my-connect \
      --product-name "My Connect" --module-prefix com.me.connect \
      --author "Ada Lovelace <ada@example.com>" \
      --base-url https://github.com/me/pkl/releases/download`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dist, err := distributionOf(cmd, args[0], fromRepository, tagSuffix,
				modulePrefix, productName, docsBaseURL, license)
			if err != nil {
				return err
			}

			versions, err := selectVersions(cmd, dist, since, limit)
			if err != nil {
				return err
			}

			if len(versions) == 0 {
				slog.Info("no release to sync")
				return nil
			}

			slog.Info("syncing", "distribution", dist.Name, "releases", len(versions),
				"oldest", versions[0], "newest", versions[len(versions)-1])

			index := &pkllib.Index{
				Distribution: dist.Name,
				Generator:    pkllib.Generator,
				ProductName:  dist.ProductName,
				License:      dist.License,
			}

			written := 0

			for _, version := range versions {
				data, err := schemaOf(cmd, dist, root, version, prune, dryRun)
				if err != nil {
					return err
				}

				if !dryRun {
					opts := fromDistribution(dist, version)
					opts.BaseURL = baseURL
					opts.SourceCode = sourceCode
					opts.IssueTracker = issueTracker
					opts.Author = author

					library, err := buildLibrary(data, opts)
					if err != nil {
						return fmt.Errorf("generate the library of %s: %w", version, err)
					}

					path := pkllib.LibraryPath(root, dist.Name, version)

					if err := osutil.ReplaceDirContents(path, library); err != nil {
						return fmt.Errorf("write the library of %s: %w", version, err)
					}
				}

				written++

				slog.Debug("wrote a library", "release", version)

				index.Releases = append(index.Releases, version)
			}

			if dryRun {
				slog.Info("dry run, wrote nothing",
					"libraries", written, "releases", len(index.Releases))
				return nil
			}

			if err := pkllib.WriteIndex(root, index); err != nil {
				return err
			}

			if err := pruneTree(root, dist.Name, index); err != nil {
				return err
			}

			slog.Info("done", "libraries", written, "releases", len(index.Releases))

			return nil
		},
	}

	flags := cmd.Flags()

	flags.StringVar(&root, "root", ".", "Directory that holds the schemas and the libraries.")
	flags.StringVar(&fromRepository, "from-repository", "",
		"Repository that holds the image of every release of a build of your own, with no tag, such as "+
			"\"ghcr.io/me/my-connect\". Give it to sync a distribution that this program does not know, "+
			"and NAME then files the output.")
	flags.StringVar(&tagSuffix, "tag-suffix", "",
		"Text that follows the version in a tag, such as \"-cloud\". Leave it empty for a tag of three numbers alone.")
	flags.StringVar(&modulePrefix, "module-prefix", "",
		"Prefix for the name of each generated module. A known NAME gives its own default.")
	flags.StringVar(&productName, "product-name", "",
		"Name of the build that reads the configuration. A known NAME gives its own default.")
	flags.StringVar(&docsBaseURL, "docs-base-url", "",
		"Root of the documentation site of the build. A known NAME gives its own default.")
	flags.StringVar(&author, "author", "",
		"Maintainer of the packages, as an RFC5322 mailbox, such as \"Ada Lovelace <ada@example.com>\". Required.")
	flags.StringVar(&license, "license", "",
		"SPDX identifier of the license of the build, such as \"Apache-2.0\". A known NAME gives its own default.")
	flags.StringVar(&baseURL, "base-url", "",
		"Directory that holds the published artifacts, such as "+
			"\"https://github.com/OWNER/REPO/releases/download\". A package writes its metadata to "+
			"BASE/TAG/TAG and its modules to BASE/TAG/TAG.zip, which is the layout that a GitHub release gives.")
	flags.StringVar(&sourceCode, "source-code", "", "URL of the source of the project.")
	flags.StringVar(&issueTracker, "issue-tracker", "", "URL of the place to report a problem.")
	flags.StringVar(&since, "since", "", "Ignore a release older than this version.")
	flags.IntVar(&limit, "limit", defaultLimit,
		"Read the schema of no more than this many of the newest releases. Zero reads every release that the registry offers. "+
			"A release whose schema is already on disk costs nothing, and stays whatever the limit says.")
	flags.BoolVar(&prune, "prune", false,
		"Remove the image of a release once it gives up its schema. A job that runs on a fresh machine keeps its disk free this way.")
	flags.BoolVar(&dryRun, "dry-run", false, "Report what would change, and write nothing.")

	for _, name := range []string{"author", "base-url"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}

	return cmd
}

// selectVersions returns the releases that the tree holds after this run,
// from the oldest to the newest.
//
// --limit keeps the newest releases and no more, and --since drops everything
// older than a version. The bounds cover the whole tree, and not the work of
// one run: a release that falls outside the window leaves the tree, and
// pruneTree takes its schema and its library with it.
//
// A package that went out is not lost. It stays published under its own tag,
// and a consumer that pinned it keeps working. The tree holds what this
// repository maintains, and the releases hold what it ever published.
func selectVersions(cmd *cobra.Command, dist *distribution.Distribution, since string, limit int) ([]string, error) {
	if since != "" && !distribution.IsVersion(since) {
		return nil, fmt.Errorf("--since %q is not a version of three numbers", since)
	}

	versions, err := distribution.Versions(cmd.Context(), nil, dist)
	if err != nil {
		return nil, err
	}

	if since != "" {
		versions = slices.DeleteFunc(versions, func(version string) bool {
			return distribution.CompareVersions(version, since) < 0
		})
	}

	if limit > 0 && len(versions) > limit {
		versions = versions[len(versions)-limit:]
	}

	return versions, nil
}

// schemaOf reads the schema of a release from disk, and reads it from the
// container image of the release when the disk does not hold it yet.
func schemaOf(cmd *cobra.Command, dist *distribution.Distribution, root, version string, prune, dryRun bool) ([]byte, error) {
	path := pkllib.SchemaPath(root, dist.Name, version)

	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read the schema of %s: %w", version, err)
	}

	slog.Info("reading a schema from its image", "release", version)

	image := dist.Image(version)

	data, err = distribution.Fetch(cmd.Context(), image, cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}

	if prune {
		distribution.Remove(cmd.Context(), image, cmd.ErrOrStderr())
	}

	if dryRun {
		return data, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("make the directory of the schemas: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write the schema of %s: %w", version, err)
	}

	return data, nil
}

// pruneTree removes the schema and the library of every release that the
// index no longer names, so that the tree holds the window and nothing else.
func pruneTree(root, name string, index *pkllib.Index) error {
	keep := map[string]bool{}
	for _, release := range index.Releases {
		keep[release] = true
	}

	libraries := filepath.Join(root, pkllib.LibraryDir, name)

	if err := pruneDir(libraries, keep, func(entry os.DirEntry) string {
		if !entry.IsDir() {
			return ""
		}

		return entry.Name()
	}); err != nil {
		return err
	}

	schemas := filepath.Join(root, pkllib.SchemaDir, name)

	return pruneDir(schemas, keep, func(entry os.DirEntry) string {
		version, found := strings.CutSuffix(entry.Name(), ".json")
		if !found {
			return ""
		}

		return version
	})
}

// pruneDir removes each entry of a directory whose version the caller does not
// keep. The version of an entry comes from versionOf, which returns an empty
// string for an entry that names no release, such as the index itself.
func pruneDir(dir string, keep map[string]bool, versionOf func(os.DirEntry) string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	for _, entry := range entries {
		version := versionOf(entry)

		if version == "" || !distribution.IsVersion(version) || keep[version] {
			continue
		}

		slog.Info("removing a release that fell outside the window",
			"release", version, "path", filepath.Join(dir, entry.Name()))

		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("remove %s: %w", entry.Name(), err)
		}
	}

	return nil
}
