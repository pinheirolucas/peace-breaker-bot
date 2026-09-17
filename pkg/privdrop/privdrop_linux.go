//go:build linux

package privdrop

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// DropTo recursively chowns dir to uid:gid, then permanently drops the
// calling process's privileges to that uid:gid. It's a no-op, reporting
// false, when the process isn't running as root: there's nothing to drop
// and no permission to chown anything anyway, which is the normal case for
// every non-container invocation of this binary.
func DropTo(dir string, uid, gid int) (bool, error) {
	if os.Geteuid() != 0 {
		return false, nil
	}

	if err := chownRecursive(dir, uid, gid); err != nil {
		return false, fmt.Errorf("failed to chown %s: %w", dir, err)
	}

	// Since Go 1.16, Setgroups/Setgid/Setuid on linux apply across every OS
	// thread (via the runtime's AllThreadsSyscall), not just the calling
	// one, so this is safe to call after other goroutines are running.
	//
	// Order matters: dropping the uid first would remove permission to
	// still change the gid or supplementary groups afterward.
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return false, fmt.Errorf("failed to reset supplementary groups: %w", err)
	}

	if err := syscall.Setgid(gid); err != nil {
		return false, fmt.Errorf("failed to setgid: %w", err)
	}

	if err := syscall.Setuid(uid); err != nil {
		return false, fmt.Errorf("failed to setuid: %w", err)
	}

	return true, nil
}

func chownRecursive(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Chown(path, uid, gid)
	})
}
