from golang:1.5

RUN mkdir -p /tmp
COPY ./plugin.go /tmp/plugin.go
WORKDIR /tmp
RUN go get -d -v 
RUN go build -o bin/plugin plugin.go
RUN cp bin/plugin /bin


ENTRYPOINT ["/bin/plugin"]
