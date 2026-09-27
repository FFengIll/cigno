/*
code copy and ref to:
https://github.com/vladimirvivien/go-tar/blob/master/tartar/tartar.go
https://medium.com/learning-the-go-programming-language/working-with-compressed-tar-files-in-go-e6fe9ce4f51d

to edit members of file in tar:
https://unix.stackexchange.com/questions/281591/change-user-id-and-group-id-ownership-of-files-within-a-tarball
*/

package pkg

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	gopath "path"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

var fs = afero.NewOsFs()

type Tarball struct {
	Root        string
	PreOptions  []TarOption
	PostOptions []TarOption

	// Skip, when set, excludes context-relative paths from the archive.
	// Returning true for a directory skips its whole subtree.
	Skip func(rel string) bool
}

type TarOption func(hdr *tar.Header) error

func PathPrefixOption(prefix string) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Name = gopath.Join(prefix, hdr.Name)
		return nil
	}
}

// ReplacePrefixPath will replace the path with new one which is required
func ReplacePrefixPath(old, new string) TarOption {
	return func(hdr *tar.Header) error {
		name := hdr.Name
		if strings.HasPrefix(name, old) {
			name = strings.TrimPrefix(name, old)
			name = gopath.Join(new, name)
			hdr.Name = name
		}
		return nil
	}
}

func ChownOption(uid, gid int) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Uid = uid
		hdr.Gid = gid
		return nil
	}
}

// ChownUIDOption sets only the numeric uid, leaving gid untouched.
func ChownUIDOption(uid int) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Uid = uid
		return nil
	}
}

// ChownGIDOption sets only the numeric gid, leaving uid untouched.
func ChownGIDOption(gid int) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Gid = gid
		return nil
	}
}

func ChownNameOption(uname, gname string) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Uname = uname
		hdr.Gname = gname
		return nil
	}
}

func ChmodOption(mod int64) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Mode = mod
		return nil
	}
}

// tar walks paths to create tar file w
func (t *Tarball) tar(w io.Writer, paths []string, options ...TarOption) (err error) {
	// enable compression if file ends in .gz
	tw := tar.NewWriter(w)
	defer tw.Close()

	for _, path := range paths {
		// path must be under the build context root;
		// relative paths resolve against the root (not the process cwd)
		rootPath, _ := filepath.Abs(t.Root)
		absPath := path
		if !filepath.IsAbs(path) {
			absPath = filepath.Join(rootPath, path)
		}
		if !strings.HasPrefix(absPath, rootPath) {
			return fmt.Errorf("no such file in buildpath: %s", path)
		}

		// walk each specified path and add encountered file to tar
		walker := func(file string, finfo os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// fill in header info using func FileInfoHeader
			hdr, err := tar.FileInfoHeader(finfo, finfo.Name())
			if err != nil {
				return err
			}

			var relFilePath string
			relFilePath, err = filepath.Rel(rootPath, file)
			if err != nil {
				return err
			}

			// ensure header has relative file path
			hdr.Name = relFilePath

			// honor exclusions (.dockerignore) before any work
			if t.Skip != nil && relFilePath != "." && t.Skip(relFilePath) {
				if finfo.Mode().IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			for _, opt := range t.PreOptions {
				if err := opt(hdr); err != nil {
					return err
				}
			}

			// option modifier
			for _, opt := range options {
				if err := opt(hdr); err != nil {
					return err
				}
			}

			// ignore the root (aka. dot now)
			if relFilePath == "." {
				return nil
			}

			// if path is a dir, only header is written
			if finfo.Mode().IsDir() {
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
				return nil
			}

			// ignore symbol link
			if finfo.Mode().Type()&os.ModeSymlink > 0 {
				return nil
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			// add file to tar
			srcFile, err := os.Open(file)
			if err != nil {
				return err
			}
			defer srcFile.Close()
			_, err = io.Copy(tw, srcFile)
			return err
		}

		// build tar: walk dirs recursively, add plain files directly
		isDir, _ := afero.IsDir(fs, absPath)
		if isDir {
			if err := filepath.Walk(absPath, walker); err != nil {
				return fmt.Errorf("failed to add %s to tar: %w", path, err)
			}
		} else {
			finfo, err := os.Lstat(absPath)
			if err != nil {
				return fmt.Errorf("failed to add %s to tar: %w", path, err)
			}
			if err := walker(absPath, finfo, nil); err != nil {
				return fmt.Errorf("failed to add %s to tar: %w", path, err)
			}
		}
	}

	return nil
}

func (t *Tarball) copyFile(dst *tar.Writer, src *tar.Reader, hdr *tar.Header, options ...TarOption) error {
	// honor exclusions (whiteouts, .dockerignore-style filters)
	if t.Skip != nil && t.Skip(hdr.Name) {
		// drain the entry body so the reader stays in sync
		if _, err := io.Copy(io.Discard, src); err != nil {
			return err
		}
		return nil
	}

	// security: refuse entries attempting path traversal, then normalize
	for _, part := range strings.Split(hdr.Name, "/") {
		if part == ".." {
			return fmt.Errorf("refusing unsafe tar entry %q: path traversal is not allowed", hdr.Name)
		}
	}
	name := gopath.Clean("/" + strings.ReplaceAll(hdr.Name, "\\", "/"))
	if name == "/" || name == "." {
		return nil // the archive root itself
	}
	hdr.Name = strings.TrimPrefix(name, "/")

	// default option modifier
	for _, opt := range t.PreOptions {
		if err := opt(hdr); err != nil {
			return err
		}
	}

	// option modifier
	for _, opt := range options {
		if err := opt(hdr); err != nil {
			return err
		}
	}

	// default option modifier
	for _, opt := range t.PostOptions {
		if err := opt(hdr); err != nil {
			return err
		}
	}

	finfo := hdr.FileInfo()
	if err := dst.WriteHeader(hdr); err != nil {
		return err
	}

	// if path is a dir, dont continue
	if finfo.Mode().IsDir() {
		return nil
	} else {
		// copy
		n, err := io.Copy(dst, src)
		if err != nil {
			return err
		}

		// validate
		if n != finfo.Size() {
			return fmt.Errorf("wrote %d, want %d", n, finfo.Size())
		}
	}

	return nil
}

// Copy
func (t *Tarball) Copy(dstName string, srcName string, options ...TarOption) (err error) {
	srcFile, err := os.Open(srcName)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	return t.CopyFromReader(dstName, srcName, srcFile, options...)
}

// CopyFromReader rewrites a tar stream from r into the file dstName,
// applying options to every entry header.
func (t *Tarball) CopyFromReader(dstName string, srcName string, r io.Reader, options ...TarOption) (err error) {
	srcReader := tar.NewReader(r)

	dstFile, err := os.Create(dstName)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	dstWriter := tar.NewWriter(dstFile)
	// BUGFIX: must do close for tar writer which will do flush too.
	defer dstWriter.Close()

	// loop each segment
	for {
		hdr, err := srcReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		err = t.copyFile(dstWriter, srcReader, hdr, options...)
		if err != nil {
			return err
		}
	}

	return nil
}
