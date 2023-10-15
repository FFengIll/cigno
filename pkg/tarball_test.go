package pkg

import (
	"io"
	"io/ioutil"
	"log"
	"testing"

	"github.com/spf13/afero"
)

func TestTarball_tartar(t1 *testing.T) {
	// manual test
	fs := afero.NewOsFs()

	var options []TarOption
	options = append(options, ChownOption(0, 0))
	options = append(options, ChownNameOption("root", "root"))
	options = append(options, PathPrefixOption("usr/local/tmp/"))
	options = append(options, ReplacePrefixPath("usr/local/test/", "usr/"))

	path := "/tmp/test.tar"
	file, _ := fs.Create(path)
	w := io.Writer(file)
	tb := Tarball{}
	if err := tb.tar(w, []string{"test/data/folder"}, options...); err != nil {
		log.Fatal(err)
	}
	file.Close()

	file, _ = fs.Open(path)
	bs, _ := ioutil.ReadAll(file)
	log.Print("bytes: ", len(bs))

	// tarPath := "out.tar"
	// files := map[string]string{
	// 	"index.html": `<body>Hello!</body>`,
	// 	"lang.json":  `[{"code":"eng","tag":"English"}]`,
	// 	"songs.txt":  `Claire de la lune, The Valkyrie, Swan Lake`,
	// }

	// unit test
	type args struct {
		w    io.Writer
		path []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Tarball{}
			if err := t.tar(tt.args.w, tt.args.path); (err != nil) != tt.wantErr {
				t1.Errorf("tar() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTarball_Copy(t *testing.T) {

	var options []TarOption
	options = append(options, ChownOption(0, 0))
	options = append(options, ChownNameOption("root", "root"))
	options = append(options, PathPrefixOption("usr/local/tmp/"))
	options = append(options, ReplacePrefixPath("usr/local/test/", "usr/"))

	src := "./test/data/tarball.tar"
	dst := "./test/data/tarball-copy.tar"
	tb := NewTarball("./")

	err := tb.Copy(dst, src)
	if err != nil {
		t.Error(err)
	}
}
