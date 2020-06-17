package main

import (
	"net/http"
	"crypto/md5"
	"encoding/binary"
	"regexp"
	"fmt"
	"sync"
	"encoding/json"
	"io/ioutil"
	"encoding/hex"
)

/*
	handles:
		- GET /shrink/<urltoshrink> - returns short url
		- GET /shortcode - redirects over to the long url
*/

type UrlRecord struct {
	url string
	hits uint64
}

type shrinkMap map[uint16]*UrlRecord

type errorResp struct {
	Msg string `json:"msg"`
}

type shrinkReq struct {
	LongURL string `json:"longURL"`
}

type shrinkResp struct {
	ShortURL string `json:"shrinkURL"`
}

var (
	ShrinkMap = make (shrinkMap, 1000000)
	shrinkMapMu sync.Mutex
	scrubRegex *regexp.Regexp
)


func shrinkHandle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var shrinkReq shrinkReq
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errMsg, _ := json.Marshal(errorResp{Msg: http.StatusText(http.StatusInternalServerError)})
		w.Write(errMsg)
		return
	}
	err = json.Unmarshal(body, &shrinkReq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errMsg, _ := json.Marshal(errorResp{Msg: http.StatusText(http.StatusBadRequest)})
		w.Write(errMsg)
		return
	}
	destination := shrinkReq.LongURL
	A := md5.Sum([]byte(destination))
	code := binary.LittleEndian.Uint16(A[:2])
	shrinkMapMu.Lock()
	ShrinkMap[code] = &UrlRecord{destination, 0}
	shrinkMapMu.Unlock()
	jsonRes, _ := json.Marshal(shrinkResp{ShortURL: fmt.Sprintf("https://%s/-%04x", r.Host, code)})
	w.Write(jsonRes)
}

func indexPage(w http.ResponseWriter, r *http.Request) {
	if match := scrubRegex.MatchString(r.URL.Path); match {
		// short url - redirect it
		code := scrubRegex.FindStringSubmatch(r.URL.Path)
		var (
			err error
			decoded []byte
		)
		if len(code) > 1 {
			decoded, err = hex.DecodeString(code[1])
		}
		if err != nil || len(code) <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Code %s is invalid. Try again.", code)
			return
		}
		intCode := binary.BigEndian.Uint16(decoded)
		shrinkMapMu.Lock()
		defer shrinkMapMu.Unlock()
		ShrinkMap[intCode].hits++
		w.Header().Set("Location", ShrinkMap[intCode].url)
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}
	http.ServeFile(w, r, "index.html")
}


func main() {
	scrubRegex, _ = regexp.Compile("^/[-]([a-fA-F0-9]+)/?$")
	http.HandleFunc("/shrink/", shrinkHandle)
	http.HandleFunc("/", indexPage)
	http.ListenAndServeTLS(":443","ssl-fqdn.crt",
		"ssl-fqdn.key", nil)
}
