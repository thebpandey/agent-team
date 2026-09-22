package preparation

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Publish only complete, flushed executable bytes. The exclusive hard link
// cannot replace a concurrently created file or follow a destination symlink.
// An interrupted write leaves an unreferenced staging file, so the next install
// can retry without mistaking a partial executable for an existing installation.
func publishExecutable(dest string, data []byte) error {
	stage, err := stageExecutable(dest, data)
	if err != nil {
		return err
	}
	defer os.Remove(stage)
	return os.Link(stage, dest)
}

func stageExecutable(dest string, data []byte) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+"-stage-")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err = file.Chmod(0700); err == nil {
		var size int
		size, err = file.Write(data)
		if err == nil && size != len(data) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
