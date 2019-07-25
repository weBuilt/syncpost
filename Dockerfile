FROM golang:1.12
COPY server.go /syncpost/
RUN cd /syncpost && go build
ENTRYPOINT ["/syncpost/syncpost"]
