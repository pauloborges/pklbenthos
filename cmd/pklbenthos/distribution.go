package main

import (
	"fmt"
	"strings"

	"github.com/pauloborges/pklbenthos/internal/distribution"
	"github.com/spf13/cobra"
)

// distributionOf returns the distribution that a command works on.
//
// A NAME that this program knows brings its own image and its own naming. Any
// other NAME needs --from-repository, which holds the image of every release
// of a build of your own, and the schemas and the libraries go under NAME.
//
// A naming flag always wins, so a caller renames the library of a known
// distribution without describing the whole of it again.
func distributionOf(cmd *cobra.Command, name, fromRepository, tagSuffix,
	modulePrefix, productName, docsBaseURL, license string,
) (*distribution.Distribution, error) {
	dist, known := distribution.Lookup(name)

	switch {
	case known && fromRepository != "":
		return nil, fmt.Errorf(
			"%q is a distribution that this program knows, so it needs no --from-repository", name)

	case !known && fromRepository == "":
		return nil, fmt.Errorf(
			"unknown distribution %q: give one of %s, or give --from-repository for a build of your own",
			name, strings.Join(distribution.SortedNames(), ", "))

	case !known:
		dist = distribution.Custom(name, fromRepository, tagSuffix)
	}

	// Lookup hands back the entry of the table itself. Take a copy of it, so
	// that a flag of this run does not reach the next caller.
	out := *dist

	if cmd.Flags().Changed("module-prefix") {
		out.ModulePrefix = modulePrefix
	}
	if cmd.Flags().Changed("product-name") {
		out.ProductName = productName
	}
	if cmd.Flags().Changed("docs-base-url") {
		out.DocsBaseURL = docsBaseURL
	}
	if cmd.Flags().Changed("license") {
		out.License = license
	}

	return &out, nil
}
