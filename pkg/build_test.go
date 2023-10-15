package pkg

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"
)

const testCopyCommand = `
FROM alpine:3.16.2 as base
COPY / /test/
`

const testFromCommand = `
FROM alpine:3.16.2 as base
`

const testCopyFromCommand = `
FROM alpine:3.16.2 as base
COPY --from=tarball . /test/
`

const testFabricCommand = `
# GLOBAL ARGS
ARG DEVPOD_BIN_IMAGE
ARG IDE_IMAGE

ARG STACK_IMAGE

# FEATURE: install-devpod-bin-image
FROM ${DEVPOD_BIN_IMAGE} as devpod

# FEATURE: install-ide-image
FROM ${IDE_IMAGE} as ide

# CURRENT IMAGE
FROM ${STACK_IMAGE} as base

COPY --from=devpod --chown=root:root /usr/lib/devpod/bin/ /usr/lib/devpod/bin/
COPY --from=devpod --chown=root:root /usr/lib/devpod/config/ /usr/lib/devpod/config/
COPY --from=ide --chown=root:root /usr/lib/ide /usr/lib/ide
EOF
`

const (
	buildDir    = "./test/data"
	outFile     = "./test/output/test.tar"
	outDir      = "./test/output/"
	tarballPath = "./test/data/tarball.tar"
)

func TestEngine_Build(t *testing.T) {
	buildDir, err := filepath.Abs(buildDir)
	if err != nil {
		panic(err)
	}
	engine := Engine{
		BuildDir:     buildDir,
		BuildContext: map[string]*BuildContext{},
	}

	engine.AddTarball("tarball", tarballPath)

	type config struct {
		tag string
		cmd string
	}

	mapping := []config{
		{"test-copy-from", testCopyFromCommand},
		{"test-copy", testCopyCommand},
		{"test-from", testFromCommand},
	}

	for _, item := range mapping {
		buf := bytes.NewBufferString(item.cmd)
		reader := io.Reader(buf)
		var options []BuildOption
		options = append(options, OutFileOption(filepath.Join(outDir, item.tag+".tar"), item.tag))
		if err := engine.Build(reader, options...); err != nil {
			t.Error(err)
		}
	}
}

func TestParseDockerFile(t *testing.T) {
	buf := bytes.NewBufferString(testCopyCommand)
	reader := io.Reader(buf)
	stages, _, err := ParseDockerFile(reader)
	if err != nil {
		return
	}

	// loop
	for _, stage := range stages {
		t.Log(stage)
		for _, cmd := range stage.Commands {
			t.Log(cmd)
		}
	}
}
