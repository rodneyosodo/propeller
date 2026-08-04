package cli

import (
	"errors"
	"fmt"
	"os"
)

const (
	cmdListUse   = "list"
	cmdViewUse   = "view <id>"
	cmdDeleteUse = "delete <id>"
	cmdStartUse  = "start <id>"
	cmdStopUse   = "stop <id>"
	cmdCreateUse = "create"
)

var errFailedToWriteConfig = errors.New("failed to write config file")

// writeConfigFile writes content to path, first clearing any directory that
// Docker Compose may have auto-created at that path (bind mounts do this when
// the source file does not yet exist). A non-empty directory is left alone.
func writeConfigFile(path string, content []byte) error {
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}

	if err == nil && info.IsDir() {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("%s is a directory and could not be cleared: %w", path, err)
		}
	}

	if err := os.WriteFile(path, content, filePermission); err != nil {
		return fmt.Errorf("failed to create %s file: %w", path, err)
	}

	return nil
}
