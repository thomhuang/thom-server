package shop

import (
	"net/http"
	"strconv"
	"strings"

	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
)

// publicOrder is the buyer-facing view of an order. It deliberately omits the
// customer name, email, and every shipping field: the session id can leak
// through browser history, shared links, or logs, so it is not sufficient
// authorization for personal data. Full orders are only returned by the
// authenticated admin list.
type publicOrder struct {
	ID               string                `json:"id"`
	Status           string                `json:"status"`
	AmountTotalCents int                   `json:"amountTotalCents"`
	Currency         string                `json:"currency"`
	Lines            []*shopdata.OrderLine `json:"lines"`
	RefundedAt       string                `json:"refundedAt"`
	CreatedAt        string                `json:"createdAt"`
	UpdatedAt        string                `json:"updatedAt"`
}

// GetOrder returns the buyer-facing view of one order by its unguessable
// session id. It never includes personal data.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if sessionID == "" {
		h.responder.BadRequest(w)
		return
	}

	order, err := h.shop.GetOrderBySessionID(sessionID)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, publicOrder{
		ID:               order.ID,
		Status:           order.Status,
		AmountTotalCents: order.AmountTotalCents,
		Currency:         order.Currency,
		Lines:            order.Lines,
		RefundedAt:       order.RefundedAt,
		CreatedAt:        order.CreatedAt,
		UpdatedAt:        order.UpdatedAt,
	}, response.NoStore()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// GetOrderByViewToken returns the full order to a buyer who followed the magic
// link emailed after payment. The token is the credential, so this response
// includes the customer and shipping fields; the session-keyed confirmation
// endpoint stays redacted. The response is marked no-store so the token and the
// personal data are not cached.
func (h *Handler) GetOrderByViewToken(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		h.responder.BadRequest(w)
		return
	}

	order, err := h.shop.GetOrderByViewToken(shopdata.HashOrderViewToken(token))
	if h.responder.HandleDataError(w, err) {
		return
	}

	headers := http.Header{
		"Cache-Control":   {"no-store"},
		"Referrer-Policy": {"no-referrer"},
		"X-Robots-Tag":    {"noindex"},
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, order, headers); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// orderListResponse wraps a page of orders with the cursor for the next page.
type orderListResponse struct {
	Orders     []*shopdata.Order `json:"orders"`
	NextCursor string            `json:"nextCursor"`
}

// ListOrders returns a page of orders, newest first, for the admin view.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	limit, ok := h.readOrderListLimit(w, r)
	if !ok {
		return
	}

	cursor, ok := h.readOrderListCursor(w, r)
	if !ok {
		return
	}

	orders, next, err := h.shop.ListOrdersPage(limit, cursor)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	nextCursor := ""
	if next > 0 {
		nextCursor = strconv.Itoa(next)
	}

	err = h.responder.WriteJSON(w, http.StatusOK, orderListResponse{
		Orders:     orders,
		NextCursor: nextCursor,
	}, nil)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) readOrderListLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 20, true
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > 100 {
		h.responder.BadRequest(w)
		return 0, false
	}

	return parsed, true
}

func (h *Handler) readOrderListCursor(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if raw == "" {
		return 0, true
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		h.responder.BadRequest(w)
		return 0, false
	}

	return parsed, true
}
