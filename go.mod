module cigno

go 1.24.0

// These match the docker/docker's dependencies configured in:
// https://github.com/moby/moby/blob/v20.10.12/vendor.conf
replace (
	github.com/moby/buildkit => github.com/moby/buildkit v0.8.3
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.0-rc92
	github.com/tonistiigi/fsutil => github.com/tonistiigi/fsutil v0.0.0-20201103201449-0834f99b7b85
)

require (
	github.com/docker/docker v28.5.2+incompatible // indirect
	github.com/google/go-containerregistry v0.20.7
	github.com/moby/buildkit v0.26.3
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.4
	github.com/spf13/afero v1.15.0
	golang.org/x/sync v0.19.0 // indirect
)

require (
	github.com/containerd/stargz-snapshotter/estargz v0.18.1 // indirect
	github.com/containerd/typeurl v1.0.2 // indirect
	github.com/docker/cli v29.1.5+incompatible // indirect
	github.com/docker/distribution v2.8.3+incompatible
	github.com/docker/docker-credential-helpers v0.9.5 // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/klauspost/compress v1.18.3 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1
	github.com/vbatts/tar-split v0.12.2
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

require github.com/spf13/cobra v1.10.2

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
)
