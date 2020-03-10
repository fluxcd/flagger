FROM alpine:3.11

RUN addgroup -S flagger \
    && adduser -S -g flagger flagger \
    && apk --no-cache add ca-certificates

WORKDIR /home/flagger

COPY /bin/flagger .

RUN chown -R flagger:flagger ./

USER flagger

ENTRYPOINT ["./flagger"]

