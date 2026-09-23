FROM busybox:1.37
COPY write-logs.sh /write-logs.sh
USER 65532:65532
ENTRYPOINT ["/bin/sh", "/write-logs.sh"]
