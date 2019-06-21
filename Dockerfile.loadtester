FROM bats/bats:v1.1.0

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk --no-cache add ca-certificates curl jq

WORKDIR /home/app

RUN curl -sSLo hey "https://storage.googleapis.com/jblabs/dist/hey_linux_v0.1.2" && \
chmod +x hey && mv hey /usr/local/bin/hey

RUN curl -sSL "https://get.helm.sh/helm-v2.12.3-linux-amd64.tar.gz" | tar xvz && \
chmod +x linux-amd64/helm && mv linux-amd64/helm /usr/local/bin/helm && \
rm -rf linux-amd64

COPY ./bin/loadtester .

RUN chown -R app:app ./

USER app

ENTRYPOINT ["./loadtester"]