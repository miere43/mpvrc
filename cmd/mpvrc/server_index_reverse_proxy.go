//go:build !index_embed

package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func (s *httpServer) index() http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			u, err := url.ParseRequestURI("http://localhost:8081")
			if err != nil {
				panic(err)
			}

			pr.SetURL(u)
			pr.SetXForwarded()
		},
	}
}
