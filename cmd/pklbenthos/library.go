package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/pauloborges/pklbenthos"
	"github.com/pauloborges/pklbenthos/internal/distribution"
	"github.com/pauloborges/pklbenthos/internal/fsutil/memfs"
	"github.com/pauloborges/pklbenthos/internal/pkllib"
	"github.com/spf13/cobra"
)

// libraryOptions describes the library of one release. The generate command
// takes the fields from its flags, and the sync command takes them from the
// distribution that it works on.
type libraryOptions struct {
	// Name is the name of the build, such as "redpanda-connect". It goes in
	// front of the release in the name of the package.
	Name string

	// Release is the version of the build, such as "4.104.0".
	Release string

	// BaseURL is where the published artifacts live, SourceCode points at the
	// project, and IssueTracker at the place to report a problem.
	BaseURL      string
	SourceCode   string
	IssueTracker string

	// Author is the maintainer of the package, as an RFC5322 mailbox.
	Author string

	// License is the SPDX identifier of the license of the build, and
	// LicenseText is the text of that license. Copyright is the notice of the
	// build, for a license whose text names no holder.
	License     string
	LicenseText string
	Copyright   string

	// ModulePrefix, ProductName and DocsBaseURL shape the modules themselves.
	// See pklbenthos.Options.
	ModulePrefix string
	ProductName  string
	DocsBaseURL  string
}

// fromDistribution fills the fields that a distribution knows.
func fromDistribution(dist *distribution.Distribution, release string) libraryOptions {
	return libraryOptions{
		Name:         dist.Name,
		Release:      release,
		License:      dist.License,
		LicenseText:  dist.LicenseText(),
		Copyright:    dist.Copyright,
		ModulePrefix: dist.ModulePrefix,
		ProductName:  dist.ProductName,
		DocsBaseURL:  dist.DocsBaseURL,
	}
}

// product returns the name that the documentation gives to the build.
func (o libraryOptions) product() string {
	if o.ProductName != "" {
		return o.ProductName
	}

	return o.Name
}

// buildLibrary compiles a schema into a library that is ready to publish.
//
// The library holds one module for each component, and the three files that
// turn a directory of modules into a Pkl package: the PklProject that names
// and versions it, the LICENSE that carries the terms of the build, and the
// doc-package-info.pkl that tells Pkldoc how to document it.
func buildLibrary(schema []byte, opts libraryOptions) (fs.FS, error) {
	modules, err := pklbenthos.GenerateFromJSONV0(schema, &pklbenthos.Options{
		ModulePrefix: opts.ModulePrefix,
		ProductName:  opts.ProductName,
		DocsBaseURL:  opts.DocsBaseURL,
	})
	if err != nil {
		return nil, err
	}

	out := memfs.New()

	err = fs.WalkDir(modules, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(modules, name)
		if err != nil {
			return err
		}

		if dir := filepath.Dir(name); dir != "." {
			if err := out.MkDirAll(dir, 0o755); err != nil {
				return err
			}
		}

		return out.WriteFile(name, data, 0o644)
	})
	if err != nil {
		return nil, err
	}

	product := opts.product()

	pkg := pkllib.Package{
		Distribution: opts.Name,
		Release:      opts.Release,
		BaseURL:      opts.BaseURL,
		SourceCode:   opts.SourceCode,
		IssueTracker: opts.IssueTracker,
		License:      opts.License,
		Author:       opts.Author,
	}

	description := fmt.Sprintf("Pkl library for the configuration of %s %s.", product, opts.Release)

	if err := out.WriteFile(pkllib.ProjectFile, []byte(pkg.Project(description)), 0o644); err != nil {
		return nil, err
	}

	// The documentation in each module comes from the schema of the build, so
	// the library is a derivative work and takes the license of that build.
	license := pkllib.License(product, opts.License, opts.Copyright, opts.LicenseText, opts.SourceCode)

	if err := out.WriteFile(pkllib.LicenseFile, []byte(license), 0o644); err != nil {
		return nil, err
	}

	// Pkldoc needs a module of its own in the library, and it throws without
	// an overview.
	overview := fmt.Sprintf(
		"Every component of the build has a module of its own, and every field\n"+
			"of a component has a type. A pipeline amends `Configuration.pkl` and\n"+
			"renders as the YAML that %s reads.", product)

	docInfo := pkllib.DocPackageInfo(pkg, opts.ModulePrefix, product, overview)

	if err := out.WriteFile(pkllib.DocPackageFile, []byte(docInfo), 0o644); err != nil {
		return nil, err
	}

	return out, nil
}

// fill takes each field of a known distribution that the caller left alone.
// A flag of the caller always wins, because the caller knows more.
func fill(cmd *cobra.Command, opts *libraryOptions, known libraryOptions) {
	if opts.Name == "" {
		opts.Name = known.Name
	}
	if opts.Release == "" {
		opts.Release = known.Release
	}
	if !cmd.Flags().Changed("module-prefix") {
		opts.ModulePrefix = known.ModulePrefix
	}
	if !cmd.Flags().Changed("product-name") {
		opts.ProductName = known.ProductName
	}
	if !cmd.Flags().Changed("docs-base-url") {
		opts.DocsBaseURL = known.DocsBaseURL
	}
	if !cmd.Flags().Changed("license") {
		opts.License = known.License
		opts.LicenseText = known.LicenseText
		opts.Copyright = known.Copyright
	}
}

// releaseOf reads the version of the build out of its schema.
//
// A build writes the version that it was built at, and some write a "v" in
// front of it. The version is empty when a schema carries none, such as one
// that a program built for a test.
func releaseOf(schema []byte) string {
	var head struct {
		Version string `json:"version"`
	}

	if err := json.Unmarshal(schema, &head); err != nil {
		return ""
	}

	return strings.TrimPrefix(head.Version, "v")
}
