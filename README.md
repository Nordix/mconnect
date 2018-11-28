# mconnect

Test programs that makes many connects to a vip address.

The purpose is fast test of connectivity and load-balancing.
`mconnect` in server mode is started on all targets and then
`mconnect` in client mode is used to do multiple connects towards the
vip address.

Local test (without a vip address);

```
> mconnect -server -address [::1]:5001 &
> time mconnect -address [::1]:5001 -nconn 1000
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

An image for use in Kubernetes is uploaded to
`docker.io/nordixorg/mconnect`.  You can install it with the provided
[manifest](mconnect.yaml). The service address (ClusterIP) can then be
used to access the server;

```
# kubectl apply -f https://github.com/Nordix/mconnect/raw/master/mconnect.yaml
service/mconnect created
deployment.apps/mconnect-deployment created
# time mconnect -address mconnect.default.svc.cluster.local:5001 -nconn 1000
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

The example shows the perfect balancing for `proxy-mode=ipvs`. The
name of the service may be different on your cluster.

Output can be in `json`. The output is not formatted so for a readable
printout pipe through [jq](https://stedolan.github.io/jq/);

```
> mconnect -address 10.0.0.2:5001 -nconn 6000 -output json | jq .
{
  "hosts": {
    "mconnect-deployment-69b454c755-8mkhk": 1500,
    "mconnect-deployment-69b454c755-rsdm8": 1499,
    "mconnect-deployment-69b454c755-w95k5": 1501,
    "mconnect-deployment-69b454c755-w9w6h": 1500
  },
  "connects": 6000,
  "failed_connects": 0,
  "failed_reads": 0,
  "start_time": "2018-10-25T14:53:34.652534263+02:00",
  "timeout": 8000000000,
  "duration": 642530530
}
```


## Build

```
go get -u github.com/Nordix/mconnect
cd $GOPATH/src/github.com/Nordix/mconnect
ver=$(git rev-parse --short HEAD)
CGO_ENABLED=0 GOOS=linux go install -a \
  -ldflags "-extldflags '-static' -X main.version=$ver" \
  github.com/Nordix/mconnect/cmd/mconnect
strip $GOPATH/bin/mconnect

# Build a docker image;
docker rmi docker.io/nordixorg/mconnect:$ver
cd $GOPATH/bin
tar -cf - mconnect | docker import \
  -c 'CMD ["/mconnect", "-server", "-udp", "-address", "[::]:5001", "-k8sprobe", "[::]:8080"]' \
  - docker.io/nordixorg/mconnect:$ver
```


## Many source addresses

Some tests requires that traffic comes from many source addresses. It
is allowed to assign entire subnets to the loopback interface and we
use it for this purpose;

```
ip addr add 222.222.222.0/24 dev lo
ip -6 addr add 5000::/112 dev lo
ip -6 ro add local 5000::/112 dev lo
```

But we must also be able to use these address for traffic on other
interfaces;

```
sudo sysctl -w net.ipv4.ip_nonlocal_bind=1
sudo sysctl -w net.ipv6.ip_nonlocal_bind=1
```

Now we can let `mconnect` (and other programs that allows the source
to be specified) to use any address from the ranges assigned to the
loopback interface;

```
mconnect -address 10.0.0.2:5001 -nconn 1000 -src 222.222.222 -srcmax 254
mconnect -address [1000::2]:5001 -nconn 1000 -src 5000: -srcmax 65534
```

The implementation is a hack and works on strings; ".rnd" is added for
ipv4 and ":rnd" for ipv6. Feel free to improve.


## Kubernetes Liveness probe

When running as a server `mconnect` can start a http server to listen
to Kubernetes Liveness probes;

```
mconnect -server -address [::]:5001 -k8sprobe [::]:8080
2018/11/28 14:58:40 K8s Liveness on; http://[::]:8080/healthz
2018/11/28 14:58:40 Listen on address;  [::]:5001
```

The probe address will reply with the hostname and the callers
address;

```
curl http://[::1]:8080/healthz
your-hostname,[::1]:43652
```

A http header can be used to emulate a malfunction;

```
curl -I -H "X-Malfunction: yes" http://[::1]:8080/healthz
HTTP/1.1 500 Internal Server Error
...
```

All subsequent calls to the liveness probe address will return "500"
until the server is restarted by Kubernetes or until a call with the
`X-Malfunction` set to anything except "yes".
