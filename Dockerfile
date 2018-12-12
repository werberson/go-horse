FROM golang:latest 

ENV p /go/src/gitex.labbs.com.br/labbsr0x/sandman-acl-proxy

RUN mkdir -p ${p}
ADD . ${p}
WORKDIR ${p}
RUN go get -d ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /main .
CMD ["/app/main"]

FROM scratch
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=0 /main /
CMD ["/main"]