package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"
    "crypto/rand"
    "time"
)

type Resp struct {
    replyId string
    status  int
    header  http.Header
    body    []byte
}

type Req struct {
    replyId  string
    respChan chan Resp
}

type PostSyncManager struct {
    proxyTo            string
    syncStartMainChan  chan Req
    syncFinishMainChan chan Resp
    httpClient http.Client
}

func (m PostSyncManager) run() {
    reg := make(map[string]([]chan Resp))
    for {
	select {
	case req := <-m.syncStartMainChan:
	    reg[req.replyId] = append(reg[req.replyId], req.respChan)
	case resp := <-m.syncFinishMainChan:
	    for _, respChan := range reg[resp.replyId] {
		respChan <- resp
	    }
	    delete(reg, resp.replyId)
	}
    }
}

type FHandler func(*http.Request) Resp

func (fHandler FHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    resp := fHandler(r)
    for k, v := range resp.header {
	w.Header()[k] = v
    }
    w.WriteHeader(resp.status)
    w.Write(resp.body)
}

func (m PostSyncManager) handler(r *http.Request) Resp {
    if r.Method != "POST" {
	return errResp("bad method")
    }
    replyId := r.Header.Get("X-R-Reply")
    if replyId != "" {
	statusStr := r.Header.Get("X-R-Reply-Status")
	status, err := strconv.Atoi(statusStr)
	if err != nil {
	    return errResp(err.Error())
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
	    return errResp(err.Error())
	}
	m.syncFinishMainChan <- Resp{replyId, status, r.Header, body}
	return Resp{replyId, 200, make(http.Header), make([]byte, 0)}
    }

    url := fmt.Sprintf("%s%s", m.proxyTo, r.RequestURI)
    proxyReq, err := http.NewRequest(r.Method, url, r.Body)
    if err != nil {
	return errResp(err.Error())
    }
    proxyReq.Header = r.Header
    resp, err := m.httpClient.Do(proxyReq)
    if err != nil {
	return errResp(err.Error())
    }
    defer resp.Body.Close()

    if replyId := resp.Header.Get("X-R-Reply"); replyId == "" {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
	    return errResp(err.Error())
	}
	return Resp{"", resp.StatusCode, resp.Header, body}
    } else {
	respChan := make(chan Resp)
	m.syncStartMainChan <- Req{replyId, respChan}
	resp := <-respChan
	return resp
    }
}

func errResp(err string) Resp {
    log.Println(err)
    return Resp{"", http.StatusInternalServerError, make(http.Header), make([]byte, 0)}
}

type TestHandler struct {
    proxyTo string
    httpClient http.Client
}

func (opt TestHandler) deferredOK(idStr string) {
    time.Sleep(2 * time.Second)
    proxyReq, err := http.NewRequest("POST", opt.proxyTo, strings.NewReader("DeferredOK"))
    if err != nil {
        log.Println(err)
        return
    }
    proxyReq.Header.Set("X-R-Reply",idStr)
    proxyReq.Header.Set("X-R-Reply-Status","201")
    resp, err := opt.httpClient.Do(proxyReq)
    if err != nil {
        log.Println(err)
        return
    }
    defer resp.Body.Close()
}

func (opt TestHandler) handler(req *http.Request) Resp {
    if(req.RequestURI == "/instant"){
        return Resp{"", 200, make(http.Header), []byte("InstantOK")}
    }
    if(req.RequestURI == "/deferred"){
        sz := 10
        b := make([]byte, sz)
        _, err := rand.Read(b)
        if err != nil {
            return errResp(err.Error())
        }
        idStr := fmt.Sprintf("%X", b)
        go opt.deferredOK(idStr)
        header := http.Header{
            "X-R-Reply": []string{idStr},
        }
        return Resp{"", 200, header, []byte("DeferOK")}
    }
    return errResp(req.RequestURI)
}

func main() {
    proxyTo := os.Getenv("C4PROXY_TO")
    if !strings.Contains(proxyTo, "://") {
	log.Fatal("bad C4PROXY_TO")
    }
    httpClient := http.Client{
        Timeout: time.Second * 10,    
    }
    
    testMode := os.Getenv("C4PROXY_TEST_MODE")
    if testMode != "" {
        h := TestHandler{proxyTo,httpClient}
        log.Fatal(http.ListenAndServe(":2080", FHandler(h.handler)))
    }
    ///
    postSyncManager := PostSyncManager{proxyTo, make(chan Req), make(chan Resp), httpClient}
    go postSyncManager.run()
    ///
    log.Fatal(http.ListenAndServe(":1080", FHandler(postSyncManager.handler)))
}

/*

C4PROXY_TO=http://127.0.0.1:2080 ./proj
C4PROXY_TEST_MODE=1 C4PROXY_TO=http://127.0.0.1:1080 ./proj
time curl -v http://127.0.0.1:1080/instant -XPOST
time curl -v http://127.0.0.1:1080/deferred -XPOST

todo optimize:
defaultTransport.MaxIdleConnsPerHost = 100
myClient = &http.Client{Transport: &defaultTransport}

*/

