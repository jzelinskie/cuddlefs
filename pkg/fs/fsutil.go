package fs

import "bazil.org/fuse"

// StringsToDirents converts a slice of strings into a slice of Dirents of type
// directory.
func StringsToDirents(xs []string) []fuse.Dirent {
	entries := make([]fuse.Dirent, 0, len(xs))
	for _, x := range xs {
		entries = append(entries, fuse.Dirent{Name: x, Type: fuse.DT_Dir})

	}
	return entries
}
