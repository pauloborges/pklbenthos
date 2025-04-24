// Package distribution knows the container images that publish a
// configuration schema, and reads a schema from one of them with Docker.
//
// A release of Redpanda Connect, and a release of Benthos, each ship a
// container image that holds the command. The command writes its own schema:
//
//	docker run --rm IMAGE list --format json-full
//
// The schema that comes out is the same V0 JSON encoding that the compiler
// reads. A caller therefore gets the schema of any released version without a
// Go build of that version.
package distribution

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// licenseTexts holds the license of each distribution, copied from the file
// that the project publishes. A generated library carries documentation that
// comes from the schema of the build, so the library is a derivative work, and
// it has to carry the license and the copyright notice of that build.
//
//go:embed licenses/*.txt
var licenseTexts embed.FS

// Distribution is a build that publishes a container image for each release.
type Distribution struct {
	// Name is how a caller asks for the distribution, such as
	// "redpanda-connect".
	Name string

	// imageName is the repository of the image, without a tag.
	imageName string

	// tagSuffix goes after the version in the tag. The cloud build of Redpanda
	// Connect shares the repository of the standard build, and a suffix keeps
	// the two apart.
	tagSuffix string

	// ModulePrefix and ProductName name the Pkl library of this distribution.
	// A caller that knows the distribution therefore also knows how to name
	// the library, as with the build that this program links in.
	ModulePrefix string
	ProductName  string

	// DocsBaseURL is the root of the documentation site of the distribution.
	// The generator resolves each cross reference of the schema under it.
	DocsBaseURL string

	// License is the SPDX identifier of the license of the build, such as
	// "Apache-2.0".
	License string

	// licenseFile names the file under licenses/ that holds the text of the
	// license.
	licenseFile string

	// Copyright is the notice of the build, for a license whose text carries
	// none of its own. The Apache license is the same text for everyone, and
	// names no holder, so a work that takes Apache material has to carry the
	// notice beside it. The MIT license names its holder in its first line,
	// so a distribution under MIT leaves this empty.
	Copyright string

	// Note describes a limit of the distribution, and is empty when there is
	// none.
	Note string
}

// LicenseText returns the license of the build, with its copyright notice. It
// returns an empty string for a distribution that carries no license, such as
// one that a caller describes with flags.
func (d *Distribution) LicenseText() string {
	if d.licenseFile == "" {
		return ""
	}

	data, err := licenseTexts.ReadFile("licenses/" + d.licenseFile)
	if err != nil {
		// The file sits beside this one and go:embed reads it at build time,
		// so a failure here means the two fell out of step.
		panic("distribution " + d.Name + ": " + err.Error())
	}

	return string(data)
}

// Image returns the reference of the image of a version.
func (d *Distribution) Image(version string) string {
	return d.imageName + ":" + version + d.tagSuffix
}

// Repository returns the image of the distribution, without a tag.
func (d *Distribution) Repository() string {
	return d.imageName
}

// Custom returns a distribution that this package does not know.
//
// A build of your own publishes an image for each of its releases, the same as
// a released build does. Give the repository of that image, without a tag, and
// a name to file the schemas and the libraries under.
//
// The tag suffix goes after the version in the tag, and is empty for a build
// that gives each release a tag of three numbers alone.
func Custom(name, repository, tagSuffix string) *Distribution {
	return &Distribution{
		Name:      name,
		imageName: repository,
		tagSuffix: tagSuffix,
	}
}

// redpandaConnectDocs is the root of the documentation site of Redpanda
// Connect. The schema writes a cross reference of Antora, such as
// "xref:guides:bloblang/about.adoc[Bloblang]", and the page of that reference
// sits at this root, under the module and the page, with no extension.
//
// Benthos writes no cross reference. Its schema holds a Markdown link with a
// path of its own, so this field stays empty for that distribution.
const redpandaConnectDocs = "https://docs.redpanda.com/redpanda-connect"

// redpandaCopyright is the notice that Redpanda Data puts at the head of each
// source file of Redpanda Connect. The Apache license names no holder, so a
// work that takes Apache material carries the notice beside the license.
const redpandaCopyright = "Copyright 2024-2026 Redpanda Data, Inc."

// The known distributions.
var distributions = []*Distribution{
	{
		Name:         "redpanda-connect",
		imageName:    "docker.redpanda.com/redpandadata/connect",
		ModulePrefix: "com.redpanda.connect",
		ProductName:  "Redpanda Connect",
		DocsBaseURL:  redpandaConnectDocs,
		License:      "Apache-2.0",
		licenseFile:  "redpanda-connect.txt",
		Copyright:    redpandaCopyright,
	},
	{
		Name:         "redpanda-connect-cloud",
		imageName:    "docker.redpanda.com/redpandadata/connect",
		tagSuffix:    "-cloud",
		ModulePrefix: "com.redpanda.connect.cloud",
		ProductName:  "Redpanda Connect",
		DocsBaseURL:  redpandaConnectDocs,
		License:      "Apache-2.0",
		licenseFile:  "redpanda-connect.txt",
		Copyright:    redpandaCopyright,
	},
	{
		// Bento is the fork of Benthos that WarpStream Labs keeps. It writes
		// its documentation in Markdown, and no cross reference of Antora, so
		// it needs no root for a documentation site.
		Name:         "bento",
		imageName:    "ghcr.io/warpstreamlabs/bento",
		ModulePrefix: "com.warpstream.bento",
		ProductName:  "Bento",
		License:      "MIT",
		licenseFile:  "bento.txt",
	},
	{
		Name:      "benthos",
		imageName: "ghcr.io/benthosdev/benthos",
		// Redpanda Data now holds Benthos, and Redpanda Connect is the build
		// that carries it forward, so the two share a namespace.
		ModulePrefix: "com.redpanda.benthos",
		// The product name matches what the compiler would fall back to, so
		// that the description of a package reads the same as the
		// documentation inside it.
		ProductName: "Benthos",
		License:     "MIT",
		licenseFile: "benthos.txt",
		Note:        "no image after 4.27.0",
	},
}

// Lookup returns the distribution of a name.
func Lookup(name string) (*Distribution, bool) {
	for _, dist := range distributions {
		if dist.Name == name {
			return dist, true
		}
	}

	return nil, false
}

// Names returns the name of every distribution, in order.
func Names() []string {
	names := make([]string, 0, len(distributions))

	for _, dist := range distributions {
		names = append(names, dist.Name)
	}

	return names
}

// Describe returns one line for each distribution, for the help of a command.
func Describe() []string {
	lines := make([]string, 0, len(distributions))

	for _, dist := range distributions {
		line := fmt.Sprintf("%-24s %s", dist.Name, dist.Image("VERSION"))
		if dist.Note != "" {
			line += " (" + dist.Note + ")"
		}

		lines = append(lines, line)
	}

	return lines
}

// Resolve reads a reference of the form "DISTRIBUTION:VERSION", or a reference
// of an image.
//
// The name in front of the last colon selects a distribution when it is the
// name of one. Every other reference is the reference of an image, and the
// returned distribution is nil, because the naming of the library of an
// unknown image is not known.
func Resolve(ref string) (image string, dist *Distribution) {
	name, version, found := strings.Cut(ref, ":")
	if found && !strings.Contains(version, ":") {
		if dist, ok := Lookup(name); ok {
			return dist.Image(version), dist
		}
	}

	return ref, nil
}

// ErrNoDocker says that Docker is not on the path.
var ErrNoDocker = errors.New("docker is not on the path: the schema of a released version comes from its container image")

// Fetch runs the command of an image, and returns the schema that it writes in
// the V0 JSON encoding. It copies the progress of Docker to progress, so that
// the pull of an image does not look like a hang.
func Fetch(ctx context.Context, image string, progress io.Writer) ([]byte, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, ErrNoDocker
	}

	var stdout bytes.Buffer

	// The command of the image is the entry point, so the arguments below go
	// to it, and not to a shell.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", image,
		"list", "--format", "json-full")
	cmd.Stdout = &stdout
	cmd.Stderr = progress

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w", image, err)
	}

	data := bytes.TrimSpace(stdout.Bytes())

	if len(data) == 0 {
		return nil, fmt.Errorf("run %s: the command wrote no schema", image)
	}

	// An image of a build that does not know `list --format json-full` writes
	// something else. Say so here, and not as a decoding error later.
	if !json.Valid(data) {
		return nil, fmt.Errorf("run %s: the command wrote no JSON, so the image holds no Benthos build", image)
	}

	return data, nil
}

// Remove deletes an image from the local store of Docker.
//
// A sync reads the schema of many releases, and the image of each one weighs
// hundreds of megabytes. A caller that reads a schema once, such as a job that
// runs on a fresh machine, removes the image to keep the disk free.
//
// It reports no error. The image has already given up its schema, and a
// failure to remove it says nothing about the schema.
func Remove(ctx context.Context, image string, progress io.Writer) {
	cmd := exec.CommandContext(ctx, "docker", "rmi", "--force", image)
	cmd.Stdout = progress
	cmd.Stderr = progress

	_ = cmd.Run()
}

// SortedNames returns the name of every distribution, in order of the
// alphabet, for a message that lists the choices.
func SortedNames() []string {
	names := Names()
	slices.Sort(names)

	return names
}
