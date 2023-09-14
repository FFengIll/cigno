package pkg

import (
	"archive/tar"
	"os"
	"testing"

	"github.com/spf13/afero/tarfs"
)

func Test_tarfs(t *testing.T) {
	tarballPath := "./test/data/tarball.tar"

	var tf *os.File
	tf, err := os.OpenFile(tarballPath, os.O_RDWR, 0)
	tfs := tarfs.New(tar.NewReader(tf))

	dest := "/usr/test"
	src := "./test"
	err = tfs.Rename(src, dest)
	if err != nil {
		t.Error(err)
	}

	tf.Close()
}
