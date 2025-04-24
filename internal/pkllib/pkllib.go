// Package pkllib holds the layout of the published Pkl libraries.
//
// The repository keeps one directory for each library that differs from the
// one before it, and an index that sends every release to the library that
// serves it. A release that changes nothing in the library therefore adds no
// directory, and still has an entry in the index.
package pkllib

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The layout of the repository.
const (
	// SchemaDir holds the schema of each release, as the image wrote it.
	SchemaDir = "schemas"

	// LibraryDir holds the generated libraries.
	LibraryDir = "pkl"

	// IndexFile sits in the directory of a distribution.
	IndexFile = "versions.json"

	// ProjectFile makes a directory into a Pkl package.
	ProjectFile = "PklProject"

	// LicenseFile carries the license of the build that the library
	// describes.
	LicenseFile = "LICENSE"

	// DocPackageFile tells Pkldoc how to document a library.
	DocPackageFile = "doc-package-info.pkl"

	// DocsiteFile tells Pkldoc the title and the overview of the whole site.
	// It sits above the distributions, and not inside a library.
	DocsiteFile = "docsite-info.pkl"
)

// License returns the text of the LICENSE file of a library.
//
// A library takes the license of the build that it describes, and not the
// license of the generator. Almost everything in a library comes from the
// configuration schema of the build: the name and the type of every field, and
// the documentation of every component. The generator adds the shape of a
// module and a few fixed sentences, and the output of a generator does not
// take the license of the generator by itself.
//
// A reader of a library therefore reads the same terms as a reader of the
// build, which is what they expect. The source of pklbenthos stays under its
// own license, and the note at the head of the file says where to find it.
//
// The upstream text is empty for a build that a caller describes with flags,
// and whose license this program does not know. The file then says so, and
// points the reader at the build.
func License(product, spdx, copyright, upstream, generator string) string {
	var out strings.Builder

	fmt.Fprintf(&out, "This library describes the configuration of %s.\n\n", product)
	fmt.Fprintf(&out, "pklbenthos generated it from the configuration schema that %s\n", product)
	fmt.Fprintf(&out, "publishes. The generator is a separate work, under its own license:\n\n")
	fmt.Fprintf(&out, "    %s\n\n", generator)

	if upstream == "" {
		fmt.Fprintf(&out, "This program does not know the license of %s. Read the license\n", product)
		fmt.Fprintf(&out, "that the build itself publishes, because this library takes the same\n")
		fmt.Fprintf(&out, "terms.\n")

		return out.String()
	}

	fmt.Fprintf(&out, "This library takes the license of %s, which is %s:\n\n", product, spdx)

	// A license whose text names no holder needs the notice beside it. The
	// Apache license is the same text for everyone and names nobody.
	if copyright != "" {
		fmt.Fprintf(&out, "%s\n", indent(copyright))
	}

	fmt.Fprintf(&out, "%s", indent(upstream))

	return out.String()
}

// DocPackageInfo returns the doc-package-info.pkl file of a library, which
// tells Pkldoc how to document it.
//
// The name is the module prefix of the distribution, and not the name of the
// Pkl package. Pkldoc reads the path of a module by cutting the name of the
// doc package off the front of the module name, and it fails when the two do
// not match. A distribution therefore needs a module prefix of its own, and
// two distributions must not share one.
//
// The version is the release of the build. Pkldoc then holds one doc package
// for the distribution, with a version for each release, and a reader moves
// between them in the version picker of the site.
func DocPackageInfo(pkg Package, modulePrefix, product, overview string) string {
	var out strings.Builder

	fmt.Fprintf(&out, "/// Pkl library for the configuration of %s %s.\n", product, pkg.Release)
	fmt.Fprintf(&out, "///\n")

	for _, line := range strings.Split(strings.TrimRight(overview, "\n"), "\n") {
		if line == "" {
			fmt.Fprintf(&out, "///\n")
			continue
		}

		fmt.Fprintf(&out, "/// %s\n", line)
	}

	fmt.Fprintf(&out, "amends \"pkl:DocPackageInfo\"\n\n")
	fmt.Fprintf(&out, "name = %q\n", modulePrefix)
	fmt.Fprintf(&out, "version = %q\n", pkg.Release)

	// Pkldoc reads a module of the package by adding a path to this URI, so it
	// has to end with a slash.
	fmt.Fprintf(&out, "importUri = \"%s#/\"\n", pkg.MetadataURL())

	fmt.Fprintf(&out, "authors { %q }\n", pkg.Author)

	if pkg.SourceCode != "" {
		fmt.Fprintf(&out, "sourceCode = %q\n", pkg.SourceCode)
	}

	// Pkldoc needs an issue tracker. Fall back to the source of the project,
	// because a reader who wants to report something starts there.
	tracker := pkg.IssueTracker
	if tracker == "" {
		tracker = pkg.SourceCode
	}

	fmt.Fprintf(&out, "issueTracker = %q\n", tracker)

	return out.String()
}

// indent puts four spaces in front of each line that carries text, so that a
// reader sees where a license starts and where it ends.
func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = "    " + line
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// SchemaPath returns the path of the schema of a release.
func SchemaPath(root, distribution, version string) string {
	return filepath.Join(root, SchemaDir, distribution, version+".json")
}

// LibraryPath returns the path of a library.
func LibraryPath(root, distribution, library string) string {
	return filepath.Join(root, LibraryDir, distribution, library)
}

// IndexPath returns the path of the index of a distribution.
func IndexPath(root, distribution string) string {
	return filepath.Join(root, LibraryDir, distribution, IndexFile)
}

// Index lists every known release of a distribution.
type Index struct {
	Distribution string `json:"distribution"`

	// Generator is the version of the generator that wrote this tree. A
	// package takes it as its own version, so a reader sees which generator
	// produced the libraries beside it.
	Generator string `json:"generator"`

	// ProductName is the name that the documentation of the library gives to
	// the build, such as "Redpanda Connect". The sync command knows it, and
	// the package command needs it for the description of each package, so
	// the index carries it from the one to the other.
	ProductName string `json:"product_name,omitempty"`

	// License is the SPDX identifier of the license of the build. Sync knows
	// it, and the package command puts it in every PklProject.
	License string `json:"license,omitempty"`

	// Releases lists every release that the tree holds, from the oldest to the
	// newest. Each one has a library directory of its own.
	Releases []string `json:"releases"`
}

// ReadIndex reads the index of a distribution. A missing file gives an empty
// index, because the first run of a sync writes it.
func ReadIndex(root, distribution string) (*Index, error) {
	data, err := os.ReadFile(IndexPath(root, distribution))
	if os.IsNotExist(err) {
		return &Index{Distribution: distribution}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the index: %w", err)
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("decode the index: %w", err)
	}

	index.Distribution = distribution

	return &index, nil
}

// WriteIndex writes the index of a distribution.
func WriteIndex(root string, index *Index) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the index: %w", err)
	}

	path := IndexPath(root, index.Distribution)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("make the directory of the index: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Generator is the version of this generator, and the version of every package
// that it writes.
//
// A package holds the library of one release of one distribution. The release
// goes in the name of the package, and not in the version, because the version
// has to carry something else. Two runs of a different generator over the same
// release give different modules, and a consumer needs a version that moves
// when the modules move.
//
// Semver leaves no room for both. A fourth number is not semver at all. Build
// metadata, such as "4.104.0+gen2", compares equal to "4.104.0", so it gives a
// consumer no signal. A prerelease, such as "4.104.0-2", compares as older,
// which says the opposite of what a later run means.
//
// Raise this when the output of the generator changes. Sync compares the
// modules of each release with the ones on disk, so a run that writes no
// library tells you that the output stayed as it was.
const Generator = "1.0.0"

// Package names the Pkl package of the library of one release.
type Package struct {
	// Distribution is the name of the build, such as "redpanda-connect".
	Distribution string

	// Release is the version of the build, such as "4.104.0".
	Release string

	// BaseURL is the directory that holds the published artifacts of every
	// package, such as "https://github.com/OWNER/REPO/releases/download".
	//
	// A package writes two artifacts under it, both named after its tag:
	//
	//	BaseURL/TAG/TAG        the metadata that Pkl reads first
	//	BaseURL/TAG/TAG.zip    the modules
	//
	// A GitHub release gives that layout by itself, because the tag names the
	// release and the assets sit under it. Any host that serves those two
	// paths works the same way.
	BaseURL string

	// SourceCode and IssueTracker point a reader at the project. Either may be
	// empty, and the field then stays out of the PklProject.
	SourceCode   string
	IssueTracker string

	// License is the SPDX identifier of the license of the build, such as
	// "Apache-2.0". An empty identifier leaves the field out, for a build
	// whose license this program does not know.
	License string

	// Author is the maintainer of the package, as an RFC5322 mailbox.
	Author string
}

// Name is the name of the package. It carries the release, because the version
// carries the generator. See [Generator].
func (p Package) Name() string {
	return p.Distribution + "-" + p.Release
}

// Tag is the name of the Git tag, and of the GitHub release, that carries the
// package. Pkl reads the metadata of a package from a URL that holds this same
// name twice, once as the release and once as the asset.
func (p Package) Tag() string {
	return p.Name() + "@" + Generator
}

// BaseURI is the identity of the package. Pkl adds "@VERSION" to it and reads
// the metadata of the package from the URL that comes out.
func (p Package) BaseURI() string {
	return "package://" + withoutScheme(p.BaseURL) + "/" + p.Tag() + "/" + p.Name()
}

// ZipURL is where Pkl reads the modules of the package.
func (p Package) ZipURL() string {
	return p.MetadataURL() + ".zip"
}

// MetadataURL is where Pkl reads the metadata of the package.
func (p Package) MetadataURL() string {
	return p.BaseURL + "/" + p.Tag() + "/" + p.Tag()
}

// withoutScheme drops the scheme of a URL, because a package URI carries its
// own.
func withoutScheme(url string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if rest, found := strings.CutPrefix(url, scheme); found {
			return rest
		}
	}

	return url
}

// Project returns the PklProject file of the package.
func (p Package) Project(description string) string {
	var out strings.Builder

	fmt.Fprintf(&out, "// This file was auto-generated by pklbenthos. Do not edit it by hand.\n\n")
	fmt.Fprintf(&out, "amends \"pkl:Project\"\n\n")
	fmt.Fprintf(&out, "package {\n")
	fmt.Fprintf(&out, "  name = %q\n", p.Name())
	fmt.Fprintf(&out, "  version = %q\n", Generator)
	fmt.Fprintf(&out, "  description = %q\n", description)
	fmt.Fprintf(&out, "  baseUri = %q\n", p.BaseURI())
	fmt.Fprintf(&out, "  packageZipUrl = %q\n", p.ZipURL())
	fmt.Fprintf(&out, "  authors { %q }\n", p.Author)

	if p.SourceCode != "" {
		fmt.Fprintf(&out, "  sourceCode = %q\n", p.SourceCode)
	}
	if p.IssueTracker != "" {
		fmt.Fprintf(&out, "  issueTracker = %q\n", p.IssueTracker)
	}

	// The license of the package is the one of the build that it describes.
	// The MIT license of the generator covers the shape of a module, and the
	// LICENSE file beside this one carries both.
	if p.License != "" {
		fmt.Fprintf(&out, "  license = %q\n", p.License)
	}

	fmt.Fprintf(&out, "}\n")

	return out.String()
}
