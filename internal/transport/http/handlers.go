package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"contest/internal/service"
	"contest/pkg/util"
)

type Handler struct {
	saleSvc     *service.SaleService
	saleMutex   sync.RWMutex
	saleService *service.SaleService
}

func NewHandler(saleSvc *service.SaleService) *Handler {
	return &Handler{
		saleSvc:     saleSvc,
		saleService: saleSvc,
	}
}

func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var userID, itemID string

	userID = r.URL.Query().Get("user_id")
	itemID = r.URL.Query().Get("id")

	if userID == "" || itemID == "" {
		var req struct {
			UserID string `json:"user_id"`
			ItemID string `json:"id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fmt.Printf("Failed to decode request body: %v\n", err)
			respondError(w, http.StatusBadRequest, "invalid request format")
			return
		}
		userID = req.UserID
		itemID = req.ItemID
	}

	fmt.Printf("Received checkout request - user_id: %s, id: %s\n", userID, itemID)

	if userID == "" || itemID == "" {
		fmt.Printf("Missing parameters - user_id: %s, id: %s\n", userID, itemID)
		respondError(w, http.StatusBadRequest, "missing user_id or id")
		return
	}

	code, err := h.saleSvc.CheckoutItem(ctx, userID, itemID)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case service.ErrItemSoldOut, service.ErrUserLimitReached:
			status = http.StatusForbidden
		case service.ErrSaleNotActive:
			status = http.StatusNotFound
		}
		fmt.Printf("Checkout error: %v\n", err)
		respondError(w, status, err.Error())
		return
	}

	fmt.Printf("Checkout successful - code: %s\n", code)
	respondJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (h *Handler) handlePurchase(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	code := r.URL.Query().Get("code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "missing code")
		return
	}

	if err := util.ValidateReservationCode(code); err != nil {
		respondError(w, http.StatusBadRequest, "invalid code format")
		return
	}

	if err := h.saleSvc.PurchaseItem(ctx, code); err != nil {
		status := http.StatusInternalServerError
		switch err {
		case service.ErrInvalidCode:
			status = http.StatusBadRequest
		case service.ErrSaleNotActive:
			status = http.StatusNotFound
		}
		respondError(w, status, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (h *Handler) handleGetActiveSale(w http.ResponseWriter, r *http.Request) {
	saleID := h.saleSvc.GetCurrentSale()
	if saleID == "" {
		respondError(w, http.StatusNotFound, "no active sale")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"sale_id": saleID})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
