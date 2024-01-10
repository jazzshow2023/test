package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// Define the target server
	target := "https://api-worker.noscription.org"

	// Parse the target URL
	url, err := url.Parse(target)
	if err != nil {
		log.Fatal(err)
	}

	// Create a reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(url)

	// Start the server and listen on a specific port, e.g., 8080
	http.HandleFunc("/inscribe/postEvent", func(w http.ResponseWriter, r *http.Request) {
		// Update the headers to allow for SSL redirection
		r.URL.Host = url.Host
		r.URL.Scheme = url.Scheme
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Host = url.Host

		// Serve the request with the reverse proxy
		proxy.ServeHTTP(w, r)
	})

	log.Println("Starting proxy server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
