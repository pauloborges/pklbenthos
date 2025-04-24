// Package pklbenthos turns the configuration schema of a Benthos build into a
// Pkl library.
//
// A build registers its components in a *service.ConfigSchema. Give that
// schema to [Generate], and it returns a file system of Pkl modules that
// describe every component of the build. Write the modules to disk with
// [os.CopyFS], or serve them from memory.
//
//	import (
//		"os"
//
//		"github.com/pauloborges/pklbenthos"
//		"github.com/redpanda-data/benthos/v4/public/service"
//	)
//
//	func generate(env *service.Environment, dir string) error {
//		schema := env.FullConfigSchema("1.0.0", "")
//
//		fsys, err := pklbenthos.Generate(schema, &pklbenthos.Options{
//			ModulePrefix: "com.example.connect",
//		})
//		if err != nil {
//			return err
//		}
//
//		return os.CopyFS(dir, fsys)
//	}
//
// The generated library holds one module for each component, plus a
// Configuration module that describes the root of a pipeline configuration.
package pklbenthos

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/pauloborges/pklbenthos/internal/benthos"
	"github.com/pauloborges/pklbenthos/internal/pkl"
)

// Schema is a configuration schema of a Benthos build. The type
// *service.ConfigSchema of
// github.com/redpanda-data/benthos/v4/public/service satisfies it.
//
// The interface keeps this package free of an import of Benthos. A caller
// already holds the schema of its own build, and the compiler needs only this
// one method to read it.
type Schema interface {
	// MarshalJSONV0 returns the schema in the V0 JSON encoding.
	MarshalJSONV0() ([]byte, error)
}

// Options controls the shape of the generated library. A nil *Options, and the
// zero value of a field, select the default.
type Options struct {
	// ModulePrefix goes in front of the name of each generated module, such as
	// "com.example.connect". An empty prefix leaves the names bare.
	//
	// The prefix does not change the path of a module in the returned file
	// system, only the module name that Pkl reports.
	ModulePrefix string

	// ProductName is the name of the build that reads the configuration, such
	// as "Redpanda Connect". The documentation of the generated modules speaks
	// of the build by this name. An empty name gives "Benthos".
	ProductName string

	// DocsBaseURL is the root of the documentation site of the build, such as
	// "https://docs.redpanda.com/redpanda-connect".
	//
	// The schema of Redpanda Connect writes a cross reference of Antora, as in
	// "xref:guides:bloblang/about.adoc[Bloblang]". A reader of a Pkl module
	// has no Antora to resolve it, so the generator turns each one into a
	// Markdown link under this root. An empty URL leaves a cross reference as
	// the schema wrote it.
	DocsBaseURL string
}

func (o *Options) compileOptions() *benthos.CompileOptions {
	if o == nil {
		return &benthos.CompileOptions{}
	}

	return &benthos.CompileOptions{
		ModulePrefix: o.ModulePrefix,
		ProductName:  o.ProductName,
		DocsBaseURL:  o.DocsBaseURL,
	}
}

// Generate returns a file system that holds the Pkl library of the schema.
//
// The paths in the file system are relative to the root of the library, such
// as "Configuration.pkl" and "inputs/Kafka.pkl".
func Generate(schema Schema, opts *Options) (fs.FS, error) {
	if schema == nil {
		return nil, errors.New("generate Pkl library: no schema")
	}

	data, err := schema.MarshalJSONV0()
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}

	return GenerateFromJSONV0(data, opts)
}

// GenerateFromJSONV0 is [Generate] for a schema that is already in the V0 JSON
// encoding, such as the output of `redpanda-connect list --format json-full`.
func GenerateFromJSONV0(data []byte, opts *Options) (fs.FS, error) {
	schema, err := benthos.ParseSchema(data)
	if err != nil {
		return nil, err
	}

	modules, err := benthos.Compile(schema, opts.compileOptions())
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}

	fsys, err := pkl.Render(modules)
	if err != nil {
		return nil, fmt.Errorf("render Pkl modules: %w", err)
	}

	return fsys, nil
}
