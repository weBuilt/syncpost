# syncpost
proxy to turn async http service to sync

syncpost is a proxy, that stands in front of a service.
It listens on 1080 port, receives POST requests and forwards them to the address passed in environment like:
C4PROXY_TO=http://127.0.0.1:2080
If service response does not contain "X-R-Reply" header, then proxy passes the response instantly to the client,
else proxy will wait for the POST request with the same value of "X-R-Reply" header,
and treat that request as a response for the initial request with status in "X-R-Reply-Status" header.

So the service is free to have minimal HTTP handler, that only creates some job.
The job can be executed later by other part of the service, then results be posted to the proxy.