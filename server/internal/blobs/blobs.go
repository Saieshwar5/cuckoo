// Package blobs stores the bytes of uploaded files.
//
// It is deliberately the smallest interface that a folder on a disk and a
// bucket in a cloud can both satisfy: put a key, open a key, delete a key.
// Nothing above it knows which is underneath.
//
// The hub streams uploads through itself rather than handing out presigned
// URLs to a storage service. On one server that costs nothing — the bytes
// cross the network the same number of times either way — and it keeps a
// hub to a single process with a single public address, with
// no second host to give a certificate to. The interface is what leaves
// presigned URLs open later: an implementation may redirect instead.
package blobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned by Open and Delete when the key holds nothing.
var ErrNotFound = errors.New("blobs: not found")

// Store is where the bytes live.
type Store interface {
	// Put writes r under key, replacing anything already there.
	Put(ctx context.Context, key string, r io.Reader) error
	// Open returns the bytes at key. The caller closes it.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the bytes at key. Removing what is not there is not
	// an error: the caller wanted it gone, and it is gone.
	Delete(ctx context.Context, key string) error
}

// Disk keeps blobs in a directory tree.
type Disk struct{ root string }

// NewDisk prepares a directory to hold blobs.
func NewDisk(root string) (*Disk, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("blobs: resolve %s: %w", root, err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("blobs: create %s: %w", abs, err)
	}
	return &Disk{root: abs}, nil
}

// Root is the directory blobs are written under.
func (d *Disk) Root() string { return d.root }

// Put writes the bytes to a temporary file and renames it into place, so a
// reader never sees half a file and a crash mid-write leaves no partial
// blob under a key that claims to be complete.
func (d *Disk) Put(ctx context.Context, key string, r io.Reader) error {
	path, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("blobs: create directory for %s: %w", key, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return fmt.Errorf("blobs: create temporary file for %s: %w", key, err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		return fmt.Errorf("blobs: write %s: %w", key, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("blobs: flush %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("blobs: close %s: %w", key, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("blobs: store %s: %w", key, err)
	}
	return ctx.Err()
}

func (d *Disk) Open(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := d.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("blobs: open %s: %w", key, err)
	}
	return f, nil
}

func (d *Disk) Delete(_ context.Context, key string) error {
	path, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("blobs: delete %s: %w", key, err)
	}
	return nil
}

// path turns a key into a file inside the root.
//
// Keys are made by this server, never by a caller, but a key that escaped
// the root would be the whole filesystem readable over HTTP, so the check
// is here rather than in whoever remembers to do it.
func (d *Disk) path(key string) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	path := filepath.Join(d.root, filepath.FromSlash(key))
	if path != d.root && !strings.HasPrefix(path, d.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("blobs: key %q escapes the root", key)
	}
	return path, nil
}

// checkKey rejects anything this server would not have generated. Disk needs
// it because a key that escaped the root would be the filesystem readable
// over HTTP; S3 has no such hole, but one rule both stores apply is what
// keeps them interchangeable — a key either accepts, or neither does.
func checkKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || !validKey(key) {
		return fmt.Errorf("blobs: invalid key %q", key)
	}
	return nil
}

// validKey allows what this server generates and nothing that could mean
// something to a filesystem: no "..", no backslashes, no control characters.
func validKey(key string) bool {
	if strings.Contains(key, "//") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}
