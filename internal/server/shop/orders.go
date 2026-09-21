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

// releaseHoldResponse reports whether the release changed anything, so the admin
// UI can tell "released" from "already gone".
type releaseHoldResponse struct {
	Released bool `json:"released"`
}

// ReleaseOrderHold frees the stock an abandoned checkout is holding. It expires
// the Stripe session first so the session can no longer be paid, then marks the
// order expired and restores its stock. Releasing an order that is no longer
// pending is a no-op, so a repeated click is safe.
func (h *Handler) ReleaseOrderHold(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if sessionID == "" {
		h.responder.BadRequest(w)
		return
	}

	order, err := h.shop.GetOrderBySessionID(sessionID)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if order.Status != shopdata.OrderStatusPending {
		if err = h.responder.WriteJSON(w, http.StatusOK, releaseHoldResponse{}, nil); err != nil {
			h.responder.ServerError(w, err)
		}
		return
	}

	if h.stripe != nil {
		if err = h.stripe.ExpireCheckoutSession(r.Context(), sessionID); err != nil {
			h.infoLog.Printf("failed to expire session %s while releasing its hold: %v", sessionID, err)
		}
	}

	changed, err := h.shop.MarkOrderExpired(sessionID)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
	if changed && order.StockReserved == 1 {
		h.restoreStock(order, order.Lines)
	}

	h.infoLog.Printf("RELEASE_HOLD session=%s order=%s changed=%t", sessionID, order.ID, changed)

	if err = h.responder.WriteJSON(w, http.StatusOK, releaseHoldResponse{Released: changed}, nil); err != nil {
		h.responder.ServerError(w, err)
	}
}
