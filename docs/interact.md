# Interact 交互式构建

```shell

# create a empty image object
# this will create a folder to store cache, and steps will be dumped into a file 
id=$(cigno new)
# set base image
cigno $uid from base.image
# rebase layer from other image
# env will be merged
# cmd will be modify if possible
cigno $uid rebase other.image:tag
# copy local files
cigno $uid copy a/b/c/ /usr/local/bin/
# for copy from, `*` is a special path which means copy all upon the base (annotation)
# only blob data is used which means no cmd, entrypoint and so on.
cigno $uid context --tarball file.tar context_name  
cigno $uid copy --from=context_name / /usr/local/bin/
# ...
# each step will save a cache image with uuid
# aka. we use registry as a cache center
# ...
# done and do anything via crane (aka. late task)
# set `annotation` for base image

# no action will do until push, aka. only push will run all step commands above
cigno $uid push -t tag
cigno $uid cleanup
```