# mconnect

Test programs that makes many connects to a vip address.

The purpose is fast test of connectivity and load-balancing.
`mconnect` in server mode is started on all targets and then
`mconnect` in client mode is used to do multiple connects towards the
vip address.

Local test;

```
> mconnect -server -address [::1]:5001 &
> time mconnect -address [::1]:5001 -nconn 1000
2018/09/25 10:30:22 Using timeout; 3s
Failed connects; 0
Failed reads; 0
your-hostname 1000
real    0m0.049s
user    0m0.070s
sys     0m0.143s
```

Even though `mconnect` is pretty fast it is not a performance
measurement tool since some bottlenecks are likely in `mconnect`
itself.

## Kubernetes

If `mconnect` in server mode is started as a Deployment with many
replicas in a Kubernetes cluster `mconnect` can be used to access the
service address (ClusterIP);

```
# time mconnect -address mconnect.default.svc.cluster.local:5001 -nconn 1000
2018/09/21 08:53:21 Using timeout; 3s
Failed connects; 0
Failed reads; 0
mconnect-deployment-5897ffb75c-dbgt5 250
mconnect-deployment-5897ffb75c-25cgp 250
mconnect-deployment-5897ffb75c-hl5cp 250
mconnect-deployment-5897ffb75c-gjt5m 250
real    0m 0.16s
user    0m 0.03s
sys     0m 0.13s
#
```

The example shows the perfect balancing for `proxy-mode=ipvs`.


## Build

```
go get github.com/Nordix/mconnect
CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' \
  github.com/Nordix/mconnect/cmd/mconnect
strip $GOPATH/bin/mconnect
```
