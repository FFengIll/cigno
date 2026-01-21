username="g_devpod"
password="919e57eec5c611ec80d4badba7b19de2"
crane auth login mirror.site -u $username -p $password

image="ide-node"
tag="20221207-1419"
prev=$image:$tag

#tag="middle-$(date "+%Y%m%d-%H%M%S")"
#crane rebase $image --new_base --old_base --tag $image:$tag
#prev=$image:$tag

alias tar=gtar

tag="manual-$(date "+%Y%m%d-%H%M%S")"
dest="usr/lib/ide/"
file="case"
tar -f blob.tar --xform s,^,$dest, -c $file --mode='a+rX' --owner=0 --group=0
crane mutate $prev --append blob.tar --tag $image:$tag
