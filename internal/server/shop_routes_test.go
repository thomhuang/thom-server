package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shopdata "thom-server/internal/shop"
)

func TestShopMutatingRoutesRequireAuth(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/shop/items", body: `{}`},
		{method: http.MethodPatch, path: "/shop/items/1", body: `{}`},
		{method: http.MethodDelete, path: "/shop/items/1"},
		{method: http.MethodPost, path: "/shop/items/1/images/presign", body: `{}`},
		{method: http.MethodPost, path: "/shop/items/1/images", body: `{}`},
		{method: http.MethodDelete, path: "/shop/items/1/images/1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestShopBrowsingRoutesArePublic(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	for _, path := range []string{"/shop/items", "/shop/items/1"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Nothing is published in the fixture, so item 1 is a 404 and the
			// list is empty. Both prove the route is reachable anonymously.
			if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
				t.Fatalf("expected 200 or 404, got %d", rr.Code)
			}
		})
	}
}

func TestShopDraftsAreVisibleOnlyToAdmin(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	draft := createShopItem(t, handler, loginCookie(t, handler), `{"title":"Draft mug","priceCents":1800,"stock":2}`)
	if draft.IsPublished {
		t.Fatal("expected the created item to be a draft")
	}

	anonymous := getShopItems(t, handler, nil)
	if len(anonymous) != 0 {
		t.Fatalf("expected drafts to be hidden from anonymous callers, got %d items", len(anonymous))
	}

	asAdmin := getShopItems(t, handler, loginCookie(t, handler))
	if len(asAdmin) != 1 {
		t.Fatalf("expected the admin to see 1 draft, got %d", len(asAdmin))
	}

	// The detail route follows the same rule.
	req := httptest.NewRequest(http.MethodGet, "/shop/items/"+draft.ID, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected a draft detail request to 404 anonymously, got %d", rr.Code)
	}
}

func TestShopAdminCanManageAnItemEndToEnd(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	item := createShopItem(t, handler, cookie, `{"title":"Mug","description":"A mug","priceCents":1800,"stock":3,"isPublished":true}`)

	if item.PriceCents != 1800 {
		t.Fatalf("priceCents = %d, want 1800", item.PriceCents)
	}

	// Publishing makes it visible to anonymous callers.
	if len(getShopItems(t, handler, nil)) != 1 {
		t.Fatal("expected the published item to be publicly visible")
	}

	// Updating.
	patch := httptest.NewRequest(http.MethodPatch, "/shop/items/"+item.ID, strings.NewReader(`{"stock":9}`))
	patch.Header.Set("Origin", "http://localhost:3000")
	patch.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for update, got %d", http.StatusOK, rr.Code)
	}

	var updated shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Stock != 9 {
		t.Fatalf("stock = %d, want 9", updated.Stock)
	}

	// Deleting.
	del := httptest.NewRequest(http.MethodDelete, "/shop/items/"+item.ID, nil)
	del.Header.Set("Origin", "http://localhost:3000")
	del.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, del)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d for delete, got %d", http.StatusNoContent, rr.Code)
	}

	if len(getShopItems(t, handler, nil)) != 0 {
		t.Fatal("expected the item list to be empty after deletion")
	}
}

func TestShopPresignReportsUnavailableWithoutR2Credentials(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	item := createShopItem(t, handler, cookie, `{"title":"Mug","priceCents":1800,"stock":1,"isPublished":true}`)

	req := httptest.NewRequest(
		http.MethodPost,
		"/shop/items/"+item.ID+"/images/presign",
		strings.NewReader(`{"contentType":"image/png"}`),
	)
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// The test app configures no R2 credentials, so uploads should degrade to a
	// clear 503 rather than a 500.
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusServiceUnavailable, rr.Code, rr.Body.String())
	}
}

func TestShopDeleteItemRemovesImageRows(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	item := createShopItem(t, handler, cookie, `{"title":"Mug","priceCents":1800,"stock":1,"isPublished":true}`)

	if _, err := app.shop.DB.Exec(
		`INSERT INTO ShopItemImages (ItemID, ObjectKey) VALUES (?, ?)`,
		item.ID, "shop/"+item.ID+"/photo.jpg",
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/shop/items/"+item.ID, nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	var remaining int
	if err := app.shop.DB.QueryRow(
		`SELECT COUNT(*) FROM ShopItemImages WHERE ItemID = ?`, item.ID,
	).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expected images to cascade with the item, found %d", remaining)
	}
}

func createShopItem(t *testing.T, handler http.Handler, cookie *http.Cookie, body string) shopdata.Item {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/shop/items", strings.NewReader(body))
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d for create, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var item shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}

	return item
}

func getShopItems(t *testing.T, handler http.Handler, cookie *http.Cookie) []*shopdata.ItemSummary {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/shop/items", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var items []*shopdata.ItemSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}

	return items
}
