package play

import (
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Create a HTTP client with sensible defaults
var httpClient = &http.Client{
	// Disable redirects, some requests have endless redirect loops
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	// Set a timeout
	Timeout: time.Second * 10,
	// Disable connection pooling and allow insecure TLS
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          0, // unlimited
		MaxIdleConnsPerHost:   2048,
		MaxConnsPerHost:       2048,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

// Sends a request and consumes the response body
func sendRequest(ctx context.Context, target *url.URL, line *logLine) error {
	log.Debugf("replaying line: %v", line)
	u, err := url.Parse(line.url)
	if err != nil {
		fail()
		return errors.Wrapf(err, "error parsing URL %s", line.url)
	}

	u.Host = target.Host
	u.Scheme = target.Scheme

	request, err := http.NewRequest(line.method, u.String(), nil)
	if err != nil {
		fail()
		return errors.Wrap(err, "error creating request")
	}

	request.Header.Set("User-Agent", line.userAgent)
	request.Header.Set("Accept-Language", "en-US;q=0.9,en;q=0.8")
	request.Header.Set("Host", target.Host)
	request.Header.Set("X-Forwarded-For", line.ipclient)

	log.Debugf("Sending %s", request.URL.String())
	startTime := time.Now()
	res, err := httpClient.Do(request)
	if err != nil {
		fail()
		return errors.Wrapf(err, "error sending request for %s", u.String())
	}

	defer res.Body.Close()

	log.Debugf("Response: %s", res.Status)
	if res.StatusCode >= 200 && res.StatusCode < 400 {
		success()
	} else {
		fail()
	}
	// Discard the request body.
	// this forces the remote host to actually return all of the bytes we requested.
	io.Copy(ioutil.Discard, res.Body)

	elapsed := time.Now().Sub(startTime)
	atomic.AddUint64(&latencySinceEpochMs, uint64(elapsed/time.Millisecond))

	return nil
}
