/*
code copy and ref to:
https://github.com/vladimirvivien/go-tar/blob/master/tartar/tartar.go
https://medium.com/learning-the-go-programming-language/working-with-compressed-tar-files-in-go-e6fe9ce4f51d

to edit members of file in tar:
https://unix.stackexchange.com/questions/281591/change-user-id-and-group-id-ownership-of-files-within-a-tarball
*/

package pkg

import (
	"fmt"
	"github.com/spf13/afero"
	"io"
	"os"
	gopath "path"
	"path/filepath"
	"strings"
)

import (
	"archive/tar"
)

var fs = afero.NewOsFs()

type Tarball struct {
	Root    string
	Options []TarOption
}

type TarOption func(hdr *tar.Header) error

func PathPrefixOption(prefix string) TarOption {
	return func(hdr *tar.Header) error {
		hdr.Name = gopath.Join(prefix, hdr.Name)
		return nil
	}
}

func ReplacePrefix(old, new string) TarOption {
	return func(hdr *tar.Header) error {
		name := hdr.Name
		name = strings.TrimPrefix(name, old)
		name = gopath.Join(new, name)
		hdr.Name = name
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
func (t *Tarball) tar(tarFile io.Writer, paths []string, options ...TarOption) (err error) {
	// enable compression if file ends in .gz
	tw := tar.NewWriter(tarFile)
	defer tw.Close()

	for _, path := range paths {
		// path must under build context
		absPath, _ := filepath.Abs(filepath.Join(t.Root, path))
		if !strings.HasPrefix(absPath, t.Root) {
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
			relFilePath, err = filepath.Rel(absPath, file)
			if err != nil {
				return err
			}

			// ignore the root (aka. dot now)
			if relFilePath == "." {
				return nil
			}

			// ignore symbol link
			if finfo.Mode().Type()&os.ModeSymlink > 0 {
				return nil
			}

			// ensure header has relative file path
			hdr.Name = relFilePath

			for _, opt := range t.Options {
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

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			// if path is a dir, dont continue
			if finfo.Mode().IsDir() {
				return nil
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
