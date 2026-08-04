//go:build windows

package filewrite

func syncDirectory(string) error {
	// Windows does not provide the Unix directory-fsync durability contract.
	return nil
}
