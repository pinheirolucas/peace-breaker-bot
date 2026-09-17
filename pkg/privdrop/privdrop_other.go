//go:build !linux

package privdrop

// DropTo is a no-op outside Linux: Go's syscall package doesn't expose
// setuid/setgid there, and dropping privileges only matters for the
// root-started Docker image this package exists for.
func DropTo(dir string, uid, gid int) (bool, error) {
	return false, nil
}
