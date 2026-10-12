package productivity

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the Stripe node.

func stripeProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "Customer", Value: "customer"},
		{Name: "Charge", Value: "charge"},
		{Name: "Payment Intent", Value: "paymentIntent"},
		{Name: "Subscription", Value: "subscription"},
		{Name: "Invoice", Value: "invoice"},
	}
	ops := []base.Option{
		{Name: "Create", Value: "create"},
		{Name: "Get", Value: "get"},
		{Name: "List", Value: "list"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Resource", "resource", "customer", "Stripe resource to operate on.", resources, true),
		base.StringOpt("Operation", "operation", "create", "Operation on the resource.", ops, true),
		base.StringProp("Customer Email", "email", "", "Customer email (resource=customer, operation=create).", "", false),
		base.StringProp("Customer Name", "name", "", "Customer name (resource=customer, operation=create).", "", false),
		base.NumberProp("Amount", "amount", 0, "Amount in the smallest currency unit (e.g. cents).", false),
		base.StringProp("Currency", "currency", "usd", "3-letter currency code (lowercase).", "usd", false),
		base.StringProp("Description", "description", "", "Charge / payment intent description.", "", false),
		base.StringProp("Payment Method", "paymentMethod", "", "Payment method ID to attach (optional).", "", false),
		base.StringProp("Customer ID", "customerId", "", "Existing customer ID (for charge / subscription).", "", false),
		base.StringProp("Price ID", "priceId", "", "Recurring price ID (for subscription).", "", false),
		base.NumberProp("Limit", "limit", 10, "Max records to return (list).", false),
	}
	return append(props, base.CommonSettings()...)
}
