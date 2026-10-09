package eval

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// injectedFile records one file copied into the workspace.
type injectedFile struct {
	rel    string
	backup string      // temp copy of the file that was overwritten; "" if none
	mode   os.FileMode // mode of the overwritten file
}

// injection is an in-progress inject copy-in that can be undone.
type injection struct {
	dir       string
	files     []injectedFile
	createdDs []string // directories (relative) created for the injection, outermost first
	backupDir string
}

// injectFiles copies entries from srcDir into dir. On error everything done so
// far is undone.
func injectFiles(dir, srcDir string, entries []string) (*injection, error) {
	inj := &injection{dir: dir}
	for _, e := range entries {
		if err := inj.add(srcDir, e); err != nil {
			if rerr := inj.restore(); rerr != nil {
				err = fmt.Errorf("%w (and restoring failed: %v)", err, rerr)
			}
			return nil, err
		}
	}
	return inj, nil
}

func (inj *injection) add(srcDir, rel string) error {
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("inject %q must be a local path", rel)
	}
	rel = filepath.Clean(rel)
	data, mode, err := readInjectSource(srcDir, rel)
	if err != nil {
		return err
	}

	// Walk the destination, refusing symlinks and creating missing parents.
	parts := strings.Split(filepath.ToSlash(rel), "/")
	cur := inj.dir
	for i, p := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, p)
		info, err := os.Lstat(cur)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(cur, 0o755); err != nil {
				return fmt.Errorf("inject %q: %w", rel, err)
			}
			inj.createdDs = append(inj.createdDs, filepath.Join(parts[:i+1]...))
		case err != nil:
			return fmt.Errorf("inject %q: %w", rel, err)
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("inject %q: destination %w: %s", rel, errPathSymlink, filepath.Join(parts[:i+1]...))
		case !info.IsDir():
			return fmt.Errorf("inject %q: %s is not a directory", rel, filepath.Join(parts[:i+1]...))
		}
	}

	dst := filepath.Join(inj.dir, rel)
	rec := injectedFile{rel: rel}
	info, err := os.Lstat(dst)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("inject %q: %w", rel, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("inject %q: destination is a symlink; refusing to write through it", rel)
	case !info.Mode().IsRegular():
		return fmt.Errorf("inject %q: destination exists and is not a regular file", rel)
	case info.Size() > maxCheckFileBytes:
		return fmt.Errorf("inject %q: existing destination is larger than 10 MiB", rel)
	default:
		if rec.backup, err = inj.backupFile(dst); err != nil {
			return fmt.Errorf("inject %q: backing up existing file: %w", rel, err)
		}
		rec.mode = info.Mode().Perm()
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("inject %q: %w", rel, err)
		}
	}
	inj.files = append(inj.files, rec)
	return writeNewFile(dst, data, mode)
}

// readInjectSource reads a regular, non-symlink source file below srcDir.
func readInjectSource(srcDir, rel string) ([]byte, os.FileMode, error) {
	abs, info, err := resolveInDir(srcDir, rel)
	switch {
	case err != nil:
		return nil, 0, fmt.Errorf("inject %q: source %v", rel, err)
	case info == nil:
		return nil, 0, fmt.Errorf("inject %q: source not found under %s", rel, srcDir)
	case info.Mode()&os.ModeSymlink != 0:
		return nil, 0, fmt.Errorf("inject %q: source is a symlink, which is not allowed", rel)
	case !info.Mode().IsRegular():
		return nil, 0, fmt.Errorf("inject %q: source must be a regular file", rel)
	case info.Size() > maxCheckFileBytes:
		return nil, 0, fmt.Errorf("inject %q: source is larger than 10 MiB", rel)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, 0, fmt.Errorf("inject %q: %w", rel, err)
	}
	return data, info.Mode().Perm(), nil
}

// writeNewFile creates path exclusively (never following a symlink) with mode.
func writeNewFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func (inj *injection) backupFile(path string) (string, error) {
	if inj.backupDir == "" {
		d, err := os.MkdirTemp("", "kairon-inject-")
		if err != nil {
			return "", err
		}
		inj.backupDir = d
	}
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.CreateTemp(inj.backupDir, "bak-")
	if err != nil {
		return "", err
	}
	_, cerr := io.Copy(dst, src)
	return dst.Name(), errors.Join(cerr, dst.Close())
}

// restore undoes the injection: injected files are removed, overwritten files
// are put back and directories created for the injection are removed. Paths
// are re-resolved so a command that swapped something for a symlink cannot
// redirect the restore.
func (inj *injection) restore() error {
	var errs []error
	for i := len(inj.files) - 1; i >= 0; i-- {
		f := inj.files[i]
		dst, info, err := resolveInDir(inj.dir, f.rel)
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %q: %w", f.rel, err))
			continue
		}
		if info != nil {
			if err := os.Remove(dst); err != nil {
				errs = append(errs, fmt.Errorf("restore %q: %w", f.rel, err))
				continue
			}
		}
		if f.backup == "" {
			continue
		}
		data, err := os.ReadFile(f.backup)
		if err == nil {
			err = writeNewFile(dst, data, f.mode)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %q: %w", f.rel, err))
		}
	}
	for i := len(inj.createdDs) - 1; i >= 0; i-- {
		p := filepath.Join(inj.dir, inj.createdDs[i])
		if info, err := os.Lstat(p); err == nil && info.IsDir() {
			os.Remove(p) // only succeeds when empty; a non-empty dir is the command's own output
		}
	}
	if inj.backupDir != "" {
		os.RemoveAll(inj.backupDir)
	}
	inj.files, inj.createdDs, inj.backupDir = nil, nil, ""
	return errors.Join(errs...)
}
