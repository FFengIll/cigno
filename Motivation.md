# Motivation
Build image with artifact and simple edit as quick as possible.
Aka.
- build image quickly
- without docker daemon and so on
- no pull for base
- feasible for artifact image build
- feasible for ci pipeline has already worked with container 
- compatible with Containerfile/Dockerfile

# Feasible Solution
1. buildah: build image without daemon
2. ~~docker~~
3. kaniko: build image in k8s pod (container)
4. crane: manual operation with remote image registry (via. /api/v2)

`crane` may be the best choice for now, since
- we always use `artifact` for many components / modules (e.g. ide, xxx-bin).
- we always use `ci` pipeline to build (aka. landun stream) things in container.
- we always use the same `base image` in a long term.
- we always `build image` with little runtime process or edit (e.g. tee, sed, wget) on files/folders in scope of control.

Furthermore, we can build our own image builder to support `Dockerfile` via `crane` - though we can not support all feature, we can work well in our situations (and maybe the future extending).