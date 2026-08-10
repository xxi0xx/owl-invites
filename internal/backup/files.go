package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

func hashFile(path, manifestPath string) (FileArtifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return FileArtifact{}, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return FileArtifact{}, err
	}
	return FileArtifact{Path: manifestPath, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size}, nil
}

func copyRegularFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup only supports regular files: %s", source)
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}

func copyTree(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("uploads path is not a directory: %s", source)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("uploads snapshot refuses symbolic link: %s", relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("uploads snapshot refuses non-regular file: %s", relative)
		}
		return copyRegularFile(path, target)
	})
}

func inventoryUploads(root string) (UploadsSnapshot, error) {
	snapshot := UploadsSnapshot{Included: true, Consistency: uploadsConsistency, Files: []FileArtifact{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup bundle contains symbolic link")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("backup bundle contains non-regular upload")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		artifact, err := hashFile(path, filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		snapshot.Files = append(snapshot.Files, artifact)
		snapshot.FileCount++
		snapshot.TotalBytes += artifact.Size
		return nil
	})
	return snapshot, err
}

func uploadsEqual(expected, actual UploadsSnapshot) bool {
	return expected.Included == actual.Included &&
		expected.Consistency == actual.Consistency &&
		expected.FileCount == actual.FileCount &&
		expected.TotalBytes == actual.TotalBytes &&
		reflect.DeepEqual(expected.Files, actual.Files)
}
