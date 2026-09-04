// Package convention holds tests that enforce project-wide rules rather than
// the behaviour of any one package.
package convention

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptPath is the file-size checker, relative to the repository root.
const scriptPath = "scripts/check-file-size.sh"

// TestFileSizeLimit fails when a source file grows past the line limit.
//
// The rule is enforced as a test so that a file which has grown too long is
// caught by the same `go test ./...` that everything else runs — by a person
// before they commit, and by an agent before it reports success. A long file is
// a design signal: the package has taken on a second job and wants splitting.
func TestFileSizeLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the checker is a shell script; run it under WSL or a POSIX shell")
	}

	root := repoRoot(t)
	script := filepath.Join(root, scriptPath)

	if _, err := os.Stat(script); err != nil {
		t.Fatalf("cannot find %s: %v", scriptPath, err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("file size check failed:\n\n%s", output)
	}

	t.Logf("%s", strings.TrimSpace(string(output)))
}

// repoRoot walks up from this test file until it finds the checker script.
// Locating the root by a file that must exist is more robust than counting
// directories, which breaks the moment a package moves.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot determine working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, scriptPath)); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("walked to the filesystem root without finding %s", scriptPath)
		}
		dir = parent
	}
}
