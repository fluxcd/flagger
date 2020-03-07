FROM alpine:3.11


RUN echo 'https://repository.walmart.com/content/repositories/alpine-v38/community' > /etc/apk/repositories \
    && echo 'https://repository.walmart.com/content/repositories/alpine-v38/main' >> /etc/apk/repositories \
    && apk update && apk upgrade && apk --no-cache add \
    ca-certificates

RUN addgroup -S flagger \
     && adduser -S -g flagger flagger

#RUN addgroup -S flagger \
#    && adduser -S -g flagger flagger \
#    && apk --no-cache add ca-certificates

WORKDIR /home/flagger

COPY /bin/flagger .

RUN chown -R flagger:flagger ./

USER flagger

ENTRYPOINT ["./flagger"]

