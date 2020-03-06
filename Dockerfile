FROM scratch
COPY --chown=0:0 image/ /
CMD ["/mconnect", "-server", "-udp", "-address", "[::]:5001", "-k8sprobe", "[::]:8080"]
