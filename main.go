package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/newrelic/go-agent"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// singleJoiningSlash is copied from httputil.singleJoiningSlash method.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "OK")
}

// NewSegmentReverseProxy is adapted from the httputil.NewSingleHostReverseProxy
// method, modified to dynamically redirect to different servers (CDN or Tracking API)
// based on the incoming request, and sets the host of the request to the host of of
// the destination URL.
func NewSegmentReverseProxy(cdn *url.URL, trackingAPI *url.URL) http.Handler {
	director := func(req *http.Request) {
		// Figure out which server to redirect to based on the incoming request.
		var target *url.URL
		if strings.HasPrefix(req.URL.String(), "/v1/projects") || strings.HasPrefix(req.URL.String(), "/analytics.js/v1") {
			target = cdn
		} else {
			target = trackingAPI
		}

		targetQuery := target.RawQuery
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}

		// Set the host of the request to the host of of the destination URL.
		// See http://blog.semanticart.com/blog/2013/11/11/a-proper-api-proxy-written-in-go/.
		req.Host = req.URL.Host
	}
	return &httputil.ReverseProxy{Director: director}
}

var port = flag.String("port", "8080", "bind address")
var debug = flag.Bool("debug", false, "debug mode")

func main() {
	flag.Parse()
	config := newrelic.NewConfig(os.Getenv("NEW_RELIC_APP_NAME"), os.Getenv("NEW_RELIC_LICENSE_KEY"))
	app, err := newrelic.NewApplication(config)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	cdnURL, err := url.Parse("https://cdn.segment.com")
	if err != nil {
		log.Fatal(err)
	}
	trackingAPIURL, err := url.Parse("https://api.segment.io")
	if err != nil {
		log.Fatal(err)
	}
	proxy := NewSegmentReverseProxy(cdnURL, trackingAPIURL)
	if *debug {
		proxy = handlers.LoggingHandler(os.Stdout, proxy)
		log.Printf("serving proxy at port %v\n", *port)
	}

	http.HandleFunc("/healthcheck", healthCheck)
	http.Handle(newrelic.WrapHandle(app, "/analytics.js/", proxy))
	http.Handle(newrelic.WrapHandle(app, "/v1/", proxy))
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
