FROM alpine:3.11 as userconf

RUN addgroup -S flagger \
    && adduser -S -G flagger flagger \
    && apk --no-cache add ca-certificates

WORKDIR /home/flagger

COPY /bin/flagger .

RUN chown -R flagger:flagger ./

FROM alpine:3.11

RUN addgroup -S flagger \
    && adduser -S -G flagger flagger \
    && apk --no-cache add ca-certificates

COPY --from=userconf /home /home

USER flagger

ENTRYPOINT ["./flagger"]

