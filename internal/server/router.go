package server

import (
	"net/http"

	coffeehttp "thom-server/internal/server/coffee"
	shophttp "thom-server/internal/server/shop"
)

func (app *App) routes() *http.ServeMux {
	mux := http.NewServeMux()
	coffeeHandler := coffeehttp.New(app.coffee, app.responder, app.infoLog)
	shopHandler := shophttp.New(app.shop, app.imageStore, app.config.R2PublicBaseURL, app.responder, app.infoLog).
		WithStripe(app.stripe, shophttp.StripeSettings{
			WebhookSecret: app.config.Stripe.WebhookSecret,
			TaxEnabled:    app.config.Stripe.TaxEnabled,
			ShippingCents: app.config.Stripe.ShippingCents,
			SuccessURL:    app.config.Stripe.SuccessURL,
			CancelURL:     app.config.Stripe.CancelURL,
		}).
		WithMailer(app.mailer, app.config.Email.SiteURL).
		WithOrderNotifications(app.config.Email.NotificationEmail)

	mux.HandleFunc("GET /ping", app.ping)

	mux.HandleFunc("POST /auth/login", app.requireAllowedOrigin(app.auth.Login))
	mux.HandleFunc("POST /auth/logout", app.auth.RequireAuth(app.auth.Logout))
	mux.HandleFunc("GET /auth/me", app.auth.RequireAuth(app.auth.GetCurrentUser))

	mux.HandleFunc("GET /coffee", coffeeHandler.GetEntries)
	mux.HandleFunc("GET /coffee/roasters", coffeeHandler.GetRoasters)
	mux.HandleFunc("POST /coffee/roasters", app.auth.RequireAuth(coffeeHandler.CreateRoaster))
	mux.HandleFunc("GET /coffee/grinders", coffeeHandler.GetGrinders)
	mux.HandleFunc("POST /coffee/grinders", app.auth.RequireAuth(coffeeHandler.CreateGrinder))
	mux.HandleFunc("GET /coffee/{id}", coffeeHandler.GetEntryByID)
	mux.HandleFunc("POST /coffee", app.auth.RequireAuth(coffeeHandler.CreateEntry))
	mux.HandleFunc("PATCH /coffee/{id}", app.auth.RequireAuth(coffeeHandler.UpdateEntry))
	mux.HandleFunc("DELETE /coffee/{id}", app.auth.RequireAuth(coffeeHandler.DeleteEntry))

	mux.HandleFunc("GET /shop/items", app.auth.OptionalAuth(shopHandler.GetItems))
	mux.HandleFunc("GET /shop/items/{id}", app.auth.OptionalAuth(shopHandler.GetItemByID))
	mux.HandleFunc("GET /shop/brands", shopHandler.GetBrands)
	mux.HandleFunc("POST /shop/brands", app.auth.RequireAuth(shopHandler.CreateBrand))
	mux.HandleFunc("POST /shop/items", app.auth.RequireAuth(shopHandler.CreateItem))
	mux.HandleFunc("PATCH /shop/items", app.auth.RequireAuth(shopHandler.SetItemsPublished))
	mux.HandleFunc("PATCH /shop/items/{id}", app.auth.RequireAuth(shopHandler.UpdateItem))
	mux.HandleFunc("DELETE /shop/items/{id}", app.auth.RequireAuth(shopHandler.DeleteItem))
	mux.HandleFunc("POST /shop/items/{id}/images/presign", app.auth.RequireAuth(shopHandler.PresignImageUpload))
	mux.HandleFunc("POST /shop/items/{id}/images", app.auth.RequireAuth(shopHandler.CreateImage))
	mux.HandleFunc("DELETE /shop/items/{id}/images/{imageId}", app.auth.RequireAuth(shopHandler.DeleteImage))

	mux.HandleFunc("POST /shop/checkout", app.requireAllowedOrigin(app.limitCheckout(shopHandler.Checkout)))
	mux.HandleFunc("GET /shop/orders", app.auth.RequireAuth(shopHandler.ListOrders))
	mux.HandleFunc("POST /shop/orders/{sessionId}/release", app.auth.RequireAuth(shopHandler.ReleaseOrderHold))
	mux.HandleFunc("GET /shop/orders/view/{token}", shopHandler.GetOrderByViewToken)
	mux.HandleFunc("GET /shop/orders/{sessionId}", shopHandler.GetOrder)
	mux.HandleFunc("POST /shop/webhooks/stripe", shopHandler.StripeWebhook)

	return mux
}
