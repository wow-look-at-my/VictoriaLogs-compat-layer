package main

import (
	"flag"
	"log"
	"net/http"
	"net/url"

	"github.com/wow-look-at-my/victorialogs-compat-layer/proxy"
)

func main() {
	listen := flag.String("listen", ":8471", "address to listen on")
	backend := flag.String("backend", "http://localhost:9428", "VictoriaLogs backend URL")
	flag.Parse()

	backendURL, err := url.Parse(*backend)
	if err != nil {
		log.Fatalf("invalid backend URL %q: %v", *backend, err)
	}

	handler := proxy.NewProxy(backendURL)
	log.Printf("listening on %s, proxying to %s", *listen, backendURL)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
