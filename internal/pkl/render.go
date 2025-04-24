package pkl

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/pauloborges/pklbenthos/internal/fsutil/memfs"
	pkltemplate "github.com/pauloborges/pklbenthos/internal/pkl/template"
)

// Render writes every module to a file system, at the path that the module
// carries. The paths are relative to the root of the library.
func Render(modules Modules) (fs.FS, error) {
	t, err := pkltemplate.Load()
	if err != nil {
		return nil, fmt.Errorf("load Pkl templates: %w", err)
	}

	fsys := memfs.New()
	var b bytes.Buffer

	for _, module := range modules {
		b.Reset()
		if err := t.ExecuteTemplate(&b, pkltemplate.Module, module); err != nil {
			return nil, fmt.Errorf("render Pkl module %q: %w", module.Path, err)
		}

		err = fsys.WriteFile(module.Path, b.Bytes(), 0666)
		if err != nil {
			return nil, fmt.Errorf("write Pkl module %q: %w", module.Path, err)
		}
	}

	return fsys, nil
}
