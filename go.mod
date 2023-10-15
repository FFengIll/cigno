module devpod/cigno

go 1.20

// These match the docker/docker's dependencies configured in:
// https://github.com/moby/moby/blob/v20.10.12/vendor.conf
replace (
	github.com/moby/buildkit => github.com/moby/buildkit v0.8.3
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.0-rc92
	github.com/tonistiigi/fsutil => github.com/tonistiigi/fsutil v0.0.0-20201103201449-0834f99b7b85
)

require (
	github.com/docker/docker v20.10.14+incompatible // indirect
	github.com/google/go-containerregistry v0.8.1-0.20220507185902-82405e5dfa82
	github.com/moby/buildkit v0.9.3
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.8.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
)

require (
	github.com/containerd/stargz-snapshotter/estargz v0.11.1 // indirect
	github.com/containerd/typeurl v1.0.2 // indirect
	github.com/docker/cli v20.10.12+incompatible // indirect
	github.com/docker/distribution v2.8.0+incompatible
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/klauspost/compress v1.14.4 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20220114050600-8b9d41f48198
	github.com/vbatts/tar-split v0.11.2 // indirect
	golang.org/x/sys v0.0.0-20220328115105-d36c6a25d886 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

require github.com/spf13/cobra v1.3.0

require (
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	gotest.tools/v3 v3.0.3 // indirect
)
