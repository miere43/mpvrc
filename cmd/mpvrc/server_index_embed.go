//go:build index_embed

package main

import (
	"net/http"

	"github.com/miere43/mpvrc/front"
)

func (s *httpServer) index() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(front.IndexHTML)
	})
}
