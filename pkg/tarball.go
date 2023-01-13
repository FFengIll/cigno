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
		// path must under build context
		rootPath, _ := filepath.Abs(t.Root)
		absPath, _ := filepath.Abs(path)
		if !strings.HasPrefix(absPath, rootPath) {
			panic("no such file in buildpath")
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

			for _, opt := range t.PreOptions {
				err := opt(hdr)
				if err != nil {
					panic(err)
				}
			}

			// option modifier
			for _, opt := range options {
				err := opt(hdr)
				if err != nil {
					panic(err)
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

			{
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
				if err != nil {
					return err
				}
				return nil
			}
		}

		// build tar
		isDir, _ := afero.IsDir(fs, absPath)
		if isDir {
			if err := filepath.Walk(absPath, walker); err != nil {
				fmt.Printf("failed to add %s to tar: %s\n", path, err)
			}
		}
	}

	return nil
}

func (t *Tarball) copyFile(dst *tar.Writer, src *tar.Reader, hdr *tar.Header, options ...TarOption) error {
	// default option modifier
	for _, opt := range t.PreOptions {
		err := opt(hdr)
		if err != nil {
			panic(err)
		}
	}

	// option modifier
	for _, opt := range options {
		err := opt(hdr)
		if err != nil {
			panic(err)
		}
	}

	// default option modifier
	for _, opt := range t.PostOptions {
		err := opt(hdr)
		if err != nil {
			panic(err)
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
	srcReader := tar.NewReader(srcFile)

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
