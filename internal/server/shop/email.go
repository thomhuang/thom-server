package shop

import (
	"context"
	"fmt"
	"html"
	"strings"

	"thom-server/internal/mail"
	shopdata "thom-server/internal/shop"
)

// WithMailer attaches the transactional email sender and the site URL used to
// build the buyer's order link. A nil sender leaves order email disabled.
func (h *Handler) WithMailer(sender mail.Sender, siteURL string) *Handler {
	h.mailer = sender
	h.siteURL = strings.TrimSuffix(strings.TrimSpace(siteURL), "/")

	return h
}

// WithOrderNotifications sets the operator address that receives a copy of every
// paid order. An empty address leaves admin notifications disabled.
func (h *Handler) WithOrderNotifications(email string) *Handler {
	h.notificationEmail = strings.TrimSpace(email)

	return h
}

// sendOrderEmail mails the buyer a link to the full order. Sending is
// best-effort: the payment is already recorded, so a failure is logged rather
// than returned, which would make Stripe redeliver the webhook.
func (h *Handler) sendOrderEmail(ctx context.Context, order *shopdata.Order, viewToken string) {
	if h.mailer == nil || h.siteURL == "" || order.CustomerEmail == "" || viewToken == "" {
		return
	}

	link := h.siteURL + "/shop/order/view?token=" + viewToken
	text, markup := orderEmailBody(order, link)

	err := h.mailer.Send(ctx, mail.Message{
		To:      order.CustomerEmail,
		Subject: "Your order #" + order.ID,
		Text:    text,
		HTML:    markup,
	})
	if err != nil {
		h.infoLog.Printf("failed to send order email for %s: %v", order.ID, err)
		return
	}

	h.infoLog.Printf("ORDER_EMAIL order=%s to=%s", order.ID, order.CustomerEmail)
}

// sendOrderNotification mails the operator a copy of a paid order so a sale is
// noticed without polling the orders page. Like the buyer email it is
// best-effort: a failure is logged rather than returned, because the payment is
// already recorded and a returned error would make Stripe redeliver.
func (h *Handler) sendOrderNotification(ctx context.Context, order *shopdata.Order) {
	to := strings.TrimSpace(h.notificationEmail)
	if h.mailer == nil || to == "" {
		return
	}

	text, markup := orderNotificationBody(order, h.siteURL)

	err := h.mailer.Send(ctx, mail.Message{
		To:      to,
		Subject: "New order #" + order.ID,
		Text:    text,
		HTML:    markup,
	})
	if err != nil {
		h.infoLog.Printf("failed to send order notification for %s: %v", order.ID, err)
		return
	}

	h.infoLog.Printf("ORDER_NOTIFICATION order=%s to=%s", order.ID, to)
}

// orderEmailBody renders the order summary and link into the plain-text and
// HTML parts of one email. Titles and addresses are escaped because they are
// admin or buyer input.
func orderEmailBody(order *shopdata.Order, link string) (string, string) {
	var plain, markup strings.Builder

	plain.WriteString("Thanks for your order.\n\n")
	fmt.Fprintf(&plain, "Order #%s\n\n", order.ID)

	markup.WriteString("<p>Thanks for your order.</p>\n")
	fmt.Fprintf(&markup, "<p><strong>Order #%s</strong></p>\n", html.EscapeString(order.ID))

	writeOrderLines(&plain, &markup, order.Lines)

	amount := formatAmount(order.AmountTotalCents, order.Currency)
	fmt.Fprintf(&plain, "\nTotal: %s\n", amount)
	fmt.Fprintf(&markup, "<p>Total: %s</p>\n", html.EscapeString(amount))

	if order.ShippingAddress != "" {
		fmt.Fprintf(&plain, "\nShipping to:\n%s\n", order.ShippingAddress)
		fmt.Fprintf(&markup, "<p>Shipping to:<br>%s</p>\n", html.EscapeString(order.ShippingAddress))
	}

	fmt.Fprintf(&plain, "\nView your order: %s\n", link)
	fmt.Fprintf(&markup, "<p><a href=\"%s\">View your order</a></p>\n", html.EscapeString(link))

	return plain.String(), markup.String()
}

// orderNotificationBody renders the operator's copy of an order. Buyer-entered
// values are escaped because they are untrusted input.
func orderNotificationBody(order *shopdata.Order, siteURL string) (string, string) {
	var plain, markup strings.Builder

	plain.WriteString("New paid order.\n\n")
	fmt.Fprintf(&plain, "Order #%s\n", order.ID)

	markup.WriteString("<p>New paid order.</p>\n")
	fmt.Fprintf(&markup, "<p><strong>Order #%s</strong></p>\n", html.EscapeString(order.ID))

	customer := strings.TrimSpace(order.CustomerName)
	if order.CustomerEmail != "" {
		if customer != "" {
			customer += " <" + order.CustomerEmail + ">"
		} else {
			customer = order.CustomerEmail
		}
	}
	if customer != "" {
		fmt.Fprintf(&plain, "Customer: %s\n", customer)
		fmt.Fprintf(&markup, "<p>Customer: %s</p>\n", html.EscapeString(customer))
	}

	writeOrderLines(&plain, &markup, order.Lines)

	amount := formatAmount(order.AmountTotalCents, order.Currency)
	fmt.Fprintf(&plain, "Total: %s\n", amount)
	fmt.Fprintf(&markup, "<p>Total: %s</p>\n", html.EscapeString(amount))

	if order.ShippingAddress != "" {
		fmt.Fprintf(&plain, "\nShip to:\n%s\n", order.ShippingAddress)
		fmt.Fprintf(&markup, "<p>Ship to:<br>%s</p>\n", html.EscapeString(order.ShippingAddress))
	}

	if siteURL != "" {
		ordersLink := siteURL + "/shop/orders"
		fmt.Fprintf(&plain, "\nView orders: %s\n", ordersLink)
		fmt.Fprintf(&markup, "<p><a href=\"%s\">View orders</a></p>\n", html.EscapeString(ordersLink))
	}

	return plain.String(), markup.String()
}

// writeOrderLines appends the order's line items to both body parts.
func writeOrderLines(plain, markup *strings.Builder, lines []*shopdata.OrderLine) {
	markup.WriteString("<ul>\n")
	for _, line := range lines {
		fmt.Fprintf(plain, "%d x %s\n", line.Quantity, line.Title)
		fmt.Fprintf(
			markup,
			"<li>%d &times; %s</li>\n",
			line.Quantity,
			html.EscapeString(line.Title),
		)
	}
	markup.WriteString("</ul>\n")
}

// formatAmount renders cents as a currency string, for example "USD 18.00".
func formatAmount(cents int, currency string) string {
	return fmt.Sprintf("%s %.2f", strings.ToUpper(currency), float64(cents)/100)
}
