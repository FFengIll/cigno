crane mutate -a test=1 -a test=2 devpod-bin:cee98abb -t nouse:crane-annotation
crane config nouse:crane-annotation | jq