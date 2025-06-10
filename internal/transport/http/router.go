package http

import (
	"context"
	"net/http"

	"contest/internal/service"
)

type Server struct {
	server *http.Server
}

func NewServer(saleSvc *service.SaleService) *Server {
	handler := NewHandler(saleSvc)
	mux := http.NewServeMux()

	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handler.handleCheckout(w, r)
	})

	mux.HandleFunc("/purchase", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handler.handlePurchase(w, r)
	})

	mux.HandleFunc("/sales/active", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handler.handleGetActiveSale(w, r)
	})

	return &Server{
		server: &http.Server{
			Addr:    ":8080",
			Handler: mux,
		},
	}
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
