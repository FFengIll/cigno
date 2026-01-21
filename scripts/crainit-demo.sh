username="g_devpod"
password="919e57eec5c611ec80d4badba7b19de2"
crane auth login mirrors.tencent.com -u $username -p $password

crane mutate cloud-ide-standard:18482418-1.73.1-20221124-1604 -a "org.opencontainers.image.base.name=tlinux3.1-base:20220427-1055" -t cloud-ide-standard:cranit

crane cp stack-java:20221208-1847 stack-java:cranit

# ---

./cranit --tag ide-java:cranit build