package benthos

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pauloborges/pklbenthos/internal/pkl"
)

// generateTestLibrary writes the full generated library to a temporary
// directory and returns the path. It skips the test when the pkl command is
// not available, because the caller needs it to check the result.
func generateTestLibrary(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("the pkl command is slow to start")
	}
	if _, err := exec.LookPath("pkl"); err != nil {
		t.Skip("pkl is not in PATH")
	}

	schema := loadStandardSchema(t)

	modules, err := Compile(schema, &CompileOptions{ModulePrefix: "com.redpanda.connect"})
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	fsys, err := pkl.Render(modules)
	if err != nil {
		t.Fatalf("render modules: %v", err)
	}

	dir := t.TempDir()
	if err := os.CopyFS(dir, fsys); err != nil {
		t.Fatalf("write modules: %v", err)
	}

	return dir
}

func runPkl(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()

	// The context of the test ends when the test does, so a pkl that hangs
	// does not outlive the run.
	cmd := exec.CommandContext(t.Context(), "pkl", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()

	return string(out), err
}

// TestGeneratedLibraryParses checks every generated module with the pkl
// command. It analyses the imports rather than evaluating the modules, because
// a module with a required field has no value until a configuration sets one.
func TestGeneratedLibraryParses(t *testing.T) {
	dir := generateTestLibrary(t)

	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".pkl") {
			relative, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated library: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("the generated library holds no modules")
	}

	out, err := runPkl(t, dir, append([]string{"analyze", "imports"}, files...)...)
	if err != nil {
		t.Fatalf("pkl analyze imports failed: %v\n%s", err, out)
	}
}

// TestGeneratedConfigurationRendersYAML checks the whole path, from the schema
// to the YAML that Redpanda Connect reads.
func TestGeneratedConfigurationRendersYAML(t *testing.T) {
	dir := generateTestLibrary(t)

	// A plugin carries its fields at the top level, so a configuration names
	// the plugin once, in the import.
	example := `
amends "Configuration.pkl"

import "tracers/Jaeger.pkl"
import "metrics/Prometheus.pkl"

config {
  tracer = (Jaeger) {
    agent_address = "localhost:6831"
    sampler_param = 0.5
  }

  metrics = (Prometheus) {
    use_histogram_timing = true
  }

  shutdown_timeout = "30s"
}
`
	path := filepath.Join(dir, "example.pkl")
	if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
		t.Fatalf("write example: %v", err)
	}

	out, err := runPkl(t, dir, "eval", "-f", "yaml", "example.pkl")
	if err != nil {
		t.Fatalf("pkl eval failed: %v\n%s", err, out)
	}

	// Redpanda Connect selects a component with a single-key object, so the
	// converter has to wrap the fields of the plugin in its name.
	wants := []string{
		"tracer:",
		"  jaeger:",
		"    agent_address: localhost:6831",
		"    sampler_param: 0.5",
		"metrics:",
		"  prometheus:",
		"shutdown_timeout: 30s",
	}

	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("the rendered YAML has no %q\n%s", want, out)
		}
	}

	// The name of the plugin is hidden, so it never appears as a field of its
	// own object.
	if strings.Contains(out, "plugin:") {
		t.Errorf("the rendered YAML holds the plugin property\n%s", out)
	}

	// A field whose only default is the empty string stays out of the
	// output. The `collector_url` of the Jaeger tracer is one of them.
	if strings.Contains(out, "collector_url") {
		t.Errorf("the rendered YAML holds an empty string field\n%s", out)
	}
	if strings.Contains(out, ": ''") {
		t.Errorf("the rendered YAML holds an empty string value\n%s", out)
	}

	// A field that no one sets stays out of the output, because Pkl drops a
	// null property. The `tests` field is optional and unset here.
	if strings.Contains(out, "tests:") {
		t.Errorf("the rendered YAML holds an unset field\n%s", out)
	}
}

// TestGeneratedConfigurationRendersOnlyWhatItSets covers the rule that the
// generated modules hold no default value. A configuration renders what its
// author chose, and nothing else, so Redpanda Connect keeps control of every
// default.
func TestGeneratedConfigurationRendersOnlyWhatItSets(t *testing.T) {
	dir := generateTestLibrary(t)

	// A configuration that sets nothing renders an empty document.
	out, err := runPkl(t, dir, "eval", "-f", "yaml", "Configuration.pkl")
	if err != nil {
		t.Fatalf("pkl eval failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(out); got != "{}" {
		t.Errorf("an untouched configuration renders:\n%s", out)
	}

	// A configuration that sets one field renders that field alone.
	example := `
amends "Configuration.pkl"

config {
  http { address = "127.0.0.1:8080" }
}
`
	path := filepath.Join(dir, "example.pkl")
	if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
		t.Fatalf("write example: %v", err)
	}

	out, err = runPkl(t, dir, "eval", "-f", "yaml", "example.pkl")
	if err != nil {
		t.Fatalf("pkl eval failed: %v\n%s", err, out)
	}

	want := "http:\n  address: 127.0.0.1:8080\n"
	if strings.TrimSpace(out) != strings.TrimSpace(want) {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestGeneratedPluginWithoutFields covers a plugin that takes no
// configuration. The converter still has to give it an empty object, because
// Redpanda Connect reads the key to select the component.
func TestGeneratedPluginWithoutFields(t *testing.T) {
	dir := generateTestLibrary(t)

	example := `
amends "Configuration.pkl"

import "tracers/None.pkl"

config {
  tracer = None
}
`
	path := filepath.Join(dir, "example.pkl")
	if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
		t.Fatalf("write example: %v", err)
	}

	out, err := runPkl(t, dir, "eval", "-f", "yaml", "example.pkl")
	if err != nil {
		t.Fatalf("pkl eval failed: %v\n%s", err, out)
	}

	if want := "tracer:\n  none: {}"; !strings.Contains(out, want) {
		t.Errorf("the rendered YAML has no %q\n%s", want, out)
	}
}

// TestGeneratedValuePlugin covers a plugin that takes a single value under its
// name instead of a set of fields, such as the `mapping` processor or the
// `fallback` output. The value renders bare, with no field map around it.
func TestGeneratedValuePlugin(t *testing.T) {
	dir := generateTestLibrary(t)

	example := `
amends "Configuration.pkl"

import "outputs/Fallback.pkl"
import "outputs/Drop.pkl"
import "outputs/Stdout.pkl"
import "processors/Mapping.pkl"

config {
  pipeline {
    processors {
      (Mapping) { value = "root.id = uuid_v4()" }
    }
  }

  output = (Fallback) {
    value = new {
      (Stdout) {}
      Drop
    }
  }
}
`
	path := filepath.Join(dir, "example.pkl")
	if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
		t.Fatalf("write example: %v", err)
	}

	out, err := runPkl(t, dir, "eval", "-f", "yaml", "example.pkl")
	if err != nil {
		t.Fatalf("pkl eval failed: %v\n%s", err, out)
	}

	// The mapping is a string under the name of the plugin, and the fallback
	// is a list of outputs under the name of the plugin.
	wants := []string{
		"- mapping: root.id = uuid_v4()",
		"output:\n  fallback:\n",
		"  - stdout:",
		"  - drop: {}",
	}

	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("the rendered YAML has no %q\n%s", want, out)
		}
	}

	// The property that carries the value is a detail of the library, so it
	// never reaches the rendered file.
	if strings.Contains(out, "value:") {
		t.Errorf("the rendered YAML holds the value property\n%s", out)
	}
}

// TestGeneratedPluginWithRenamedField covers a plugin with a field whose name
// Pkl keeps for itself. The `drop_on` output has a field named `output`, so
// its module declares `yamlOutput`, which still renders as `output`.
func TestGeneratedPluginWithRenamedField(t *testing.T) {
	dir := generateTestLibrary(t)

	example := `
amends "Configuration.pkl"

import "outputs/DropOn.pkl"
import "outputs/Stdout.pkl"

config {
  output = (DropOn) {
    error = true
    yamlOutput = (Stdout) {}
  }
}
`
	path := filepath.Join(dir, "example.pkl")
	if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
		t.Fatalf("write example: %v", err)
	}

	out, err := runPkl(t, dir, "eval", "-f", "yaml", "example.pkl")
	if err != nil {
		t.Fatalf("pkl eval failed: %v\n%s", err, out)
	}

	wants := []string{
		"output:\n  drop_on:\n",
		"    error: true",
		"    output:\n      stdout:",
	}

	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("the rendered YAML has no %q\n%s", want, out)
		}
	}

	// The name that the module gives the field stays in the module.
	if strings.Contains(out, "yamlOutput") {
		t.Errorf("the rendered YAML holds the renamed property\n%s", out)
	}
}
