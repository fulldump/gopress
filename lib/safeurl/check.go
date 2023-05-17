package safeurl

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

var checkClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                  nil,
		DialContext:            nil,
		Dial:                   nil,
		DialTLS:                nil,
		TLSClientConfig:        nil,
		TLSHandshakeTimeout:    0,
		DisableKeepAlives:      false,
		DisableCompression:     false,
		MaxIdleConns:           10,
		MaxIdleConnsPerHost:    10,
		MaxConnsPerHost:        10,
		IdleConnTimeout:        time.Second * 10,
		ResponseHeaderTimeout:  time.Second * 10,
		ExpectContinueTimeout:  time.Second * 10,
		TLSNextProto:           nil,
		ProxyConnectHeader:     nil,
		MaxResponseHeaderBytes: 4 * 1024,
		WriteBufferSize:        0,
		ReadBufferSize:         0,
		ForceAttemptHTTP2:      false,
	},
	Timeout: time.Second * 5,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func AssertSafeUrl(str string) error {

	u, err := url.Parse(str)
	if err != nil {
		return err
	}

	if u.Host == "" {
		return errors.New("host is empty")
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("sheme is not valid (expected http or https)")
	}

	host := u.Host
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("lookupHost(%s):%v", host, err)
	}

	for _, a := range addrs {
		ip := net.ParseIP(a)
		err := checkPrivateIP(ip)
		if err != nil {
			return fmt.Errorf("checkPrivateIP(%s):%v", a, err)
		}
	}

	return nil
}
