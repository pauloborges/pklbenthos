package memfs

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemFS is an in-memory file system that implements fs.FS.
type MemFS struct {
	mu    sync.RWMutex
	nodes map[string]*memNode
}

// memNode represents either a file or directory in the filesystem
type memNode struct {
	data    []byte
	modTime time.Time
	mode    fs.FileMode
	isDir   bool
}

// New returns a new in-memory file system
func New() *MemFS {
	// Initialize with root directory
	nodes := make(map[string]*memNode)
	nodes["."] = &memNode{
		modTime: time.Now(),
		mode:    fs.ModeDir | 0755,
		isDir:   true,
	}

	return &MemFS{
		nodes: nodes,
	}
}

// MkDirAll creates a directory named path, along with any necessary parents.
func (m *MemFS) MkDirAll(name string, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = path.Clean(name)

	// Root directory always exists
	if name == "." {
		return nil
	}

	// If it already exists, check if it's a directory
	if node, exists := m.nodes[name]; exists {
		if node.isDir {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
	}

	// Create all parent directories
	components := strings.Split(name, "/")
	currentPath := "."

	for _, component := range components {
		if component == "" {
			continue
		}

		if currentPath == "." {
			currentPath = component
		} else {
			currentPath = path.Join(currentPath, component)
		}

		// Skip if entry already exists and is a directory
		if node, exists := m.nodes[currentPath]; exists {
			if !node.isDir {
				return &fs.PathError{Op: "mkdir", Path: currentPath, Err: fs.ErrExist}
			}
			continue
		}

		// Create the directory
		m.nodes[currentPath] = &memNode{
			modTime: time.Now(),
			mode:    perm | fs.ModeDir,
			isDir:   true,
		}
	}

	return nil
}

// WriteFile writes data to the named file, creating or overwriting it with given permissions.
func (m *MemFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = path.Clean(name)

	// Check if path exists as a directory
	if node, exists := m.nodes[name]; exists && node.isDir {
		return &fs.PathError{Op: "write", Path: name, Err: fs.ErrInvalid}
	}

	// Create parent directories if they don't exist
	dir := path.Dir(name)
	if dir != "." {
		if node, exists := m.nodes[dir]; !exists || !node.isDir {
			// Unlock to avoid deadlock when calling MkDirAll which will lock again
			m.mu.Unlock()
			err := m.MkDirAll(dir, 0755)
			m.mu.Lock()
			if err != nil {
				return err
			}
		}
	}

	// Write the file
	m.nodes[name] = &memNode{
		data:    append([]byte(nil), data...),
		modTime: time.Now(),
		mode:    perm,
		isDir:   false,
	}

	return nil
}

// ReadFile reads the named file and returns its contents.
func (m *MemFS) ReadFile(name string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = path.Clean(name)

	// Get the node
	node, ok := m.nodes[name]
	if !ok {
		return nil, fs.ErrNotExist
	}

	// Check if it's a directory
	if node.isDir {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}

	return append([]byte(nil), node.data...), nil
}

// Open implements fs.FS. It returns a File for reading.
func (m *MemFS) Open(name string) (fs.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = path.Clean(name)

	// Get the node
	node, ok := m.nodes[name]
	if !ok {
		return nil, fs.ErrNotExist
	}

	// Check if it's a directory
	if node.isDir {
		return m.openDir(name)
	}

	// It's a file
	return &fsFile{
		name:      name,
		data:      append([]byte(nil), node.data...),
		entries:   nil, // Not needed for files
		offset:    0,
		dirOffset: 0,
		modTime:   node.modTime,
		mode:      node.mode,
		isDir:     false,
	}, nil
}

// openDir creates a directory file for reading directory contents
func (m *MemFS) openDir(dirPath string) (fs.File, error) {
	// Ensure the directory exists
	dirNode, exists := m.nodes[dirPath]
	if !exists || !dirNode.isDir {
		return nil, fs.ErrNotExist
	}

	// Create list of entries in this directory
	var entries []fs.DirEntry

	// Track entries we've already added to avoid duplicates
	seenEntries := make(map[string]bool)

	// Loop through all nodes to find children of this directory
	for nodePath, node := range m.nodes {
		// Skip self
		if nodePath == dirPath {
			continue
		}

		// For the root directory
		if dirPath == "." {
			// Only include top-level entries
			if !strings.Contains(nodePath, "/") {
				entryName := nodePath
				if !seenEntries[entryName] {
					entries = append(entries, &dirEntry{
						name:    entryName,
						isDir:   node.isDir,
						mode:    node.mode,
						size:    int64(len(node.data)),
						modTime: node.modTime,
					})
					seenEntries[entryName] = true
				}
			}
		} else {
			// Check if it's a direct child of this directory
			parent := path.Dir(nodePath)
			if parent == dirPath {
				entryName := path.Base(nodePath)
				if !seenEntries[entryName] {
					entries = append(entries, &dirEntry{
						name:    entryName,
						isDir:   node.isDir,
						mode:    node.mode,
						size:    int64(len(node.data)),
						modTime: node.modTime,
					})
					seenEntries[entryName] = true
				}
			}
		}
	}

	// Sort entries by name for consistency
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return &fsFile{
		name:      dirPath,
		data:      nil, // Directories don't have data
		entries:   entries,
		offset:    0,
		dirOffset: 0,
		modTime:   dirNode.modTime,
		mode:      dirNode.mode,
		isDir:     true,
	}, nil
}

// Stat implements fs.StatFS
func (m *MemFS) Stat(name string) (fs.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = path.Clean(name)

	// Get the node
	node, ok := m.nodes[name]
	if !ok {
		return nil, fs.ErrNotExist
	}

	// Return appropriate FileInfo
	return &fileInfo{
		name:    path.Base(name),
		size:    int64(len(node.data)),
		modTime: node.modTime,
		mode:    node.mode,
		isDir:   node.isDir,
	}, nil
}

// fsFile implements fs.File and fs.ReadDirFile for both files and directories
type fsFile struct {
	name      string
	data      []byte        // nil for directories
	entries   []fs.DirEntry // nil for regular files
	offset    int64         // used for file reads
	dirOffset int           // used for directory reads
	modTime   time.Time
	mode      fs.FileMode
	isDir     bool
}

// Stat returns the metadata of the open file. It carries out part of fs.File.
func (f *fsFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:    path.Base(f.name),
		size:    int64(len(f.data)),
		modTime: f.modTime,
		mode:    f.mode,
		isDir:   f.isDir,
	}, nil
}

// Read copies the next bytes of the file. It carries out part of fs.File.
func (f *fsFile) Read(p []byte) (int, error) {
	if f.isDir {
		return 0, fs.ErrInvalid
	}

	if f.offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)
	return n, nil
}

// Close releases the open file. It carries out part of fs.File.
func (f *fsFile) Close() error {
	return nil
}

// ReadDir implements fs.ReadDirFile
func (f *fsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.isDir {
		return nil, fs.ErrInvalid
	}

	if f.dirOffset >= len(f.entries) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		result := f.entries[f.dirOffset:]
		f.dirOffset = len(f.entries)
		return result, nil
	}

	end := min(f.dirOffset+n, len(f.entries))
	result := f.entries[f.dirOffset:end]
	f.dirOffset = end

	return result, nil
}

// dirEntry implements fs.DirEntry
type dirEntry struct {
	name    string
	isDir   bool
	mode    fs.FileMode
	size    int64
	modTime time.Time
}

// Name returns the name of the entry, without its directory. It carries out
// part of fs.DirEntry.
func (d *dirEntry) Name() string {
	return d.name
}

// IsDir tells if the entry is a directory. It carries out part of fs.DirEntry.
func (d *dirEntry) IsDir() bool {
	return d.isDir
}

// Type returns the bits of the mode that name the kind of the entry. It
// carries out part of fs.DirEntry.
func (d *dirEntry) Type() fs.FileMode {
	return d.mode.Type()
}

// Info returns the metadata of the entry. It carries out part of fs.DirEntry.
func (d *dirEntry) Info() (fs.FileInfo, error) {
	return &fileInfo{
		name:    d.name,
		size:    d.size,
		modTime: d.modTime,
		mode:    d.mode,
		isDir:   d.isDir,
	}, nil
}

// fileInfo implements fs.FileInfo for a file in memory
type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	mode    fs.FileMode
	isDir   bool
}

// Name returns the name of the file, without its directory. It carries out
// part of fs.FileInfo.
func (fi *fileInfo) Name() string {
	return fi.name
}

// Size returns how many bytes the file holds. It carries out part of
// fs.FileInfo.
func (fi *fileInfo) Size() int64 {
	return fi.size
}

// Mode returns the mode of the file. It carries out part of fs.FileInfo.
func (fi *fileInfo) Mode() fs.FileMode {
	return fi.mode
}

// ModTime returns when the file last changed. It carries out part of
// fs.FileInfo.
func (fi *fileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir tells if the file is a directory. It carries out part of fs.FileInfo.
func (fi *fileInfo) IsDir() bool {
	return fi.isDir
}

// Sys returns the source of the metadata, and nil because this file system
// has none. It carries out part of fs.FileInfo.
func (fi *fileInfo) Sys() any {
	return nil
}
