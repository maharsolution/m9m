package productivity

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// stripeProperties describes the Stripe node.
//
// productivity.StripeNode reads `operation`, `apiKey`, `baseUrl`,
// `email`, `name`, `description`, `amount`, `currency`,
// `customerId` and `limit`. Each operation consumes a different
// subset, so the operation-specific fields are shown conditionally.
func stripeProperties() []base.NodeProperty {
	ops := []base.Option{
		{Name: "Create customer", Value: "createCustomer"},
		{Name: "Create payment intent", Value: "createPaymentIntent"},
		{Name: "List payments", Value: "listPayments"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Operation", "operation", "createCustomer",
			"Stripe operation to run.",
			ops, true),
		nodeprops.Secret("API key", "apiKey", "",
			"Stripe secret API key.",
			"sk_live_…", true),
		nodeprops.Str("Base URL", "baseUrl", "https://api.stripe.com/v1",
			"Stripe API root.",
			"https://api.stripe.com/v1", false),
		nodeprops.Str("Email", "email", "",
			"Customer email address.",
			"user@example.com", false).
			WithDisplayOptions(nodeprops.Show("operation", "createCustomer")),
		nodeprops.Str("Name", "name", "",
			"Customer name.",
			"Jane Doe", false).
			WithDisplayOptions(nodeprops.Show("operation", "createCustomer")),
		nodeprops.Str("Description", "description", "",
			"Description attached to the payment intent.",
			"Order 1234", false).
			WithDisplayOptions(nodeprops.Show("operation", "createPaymentIntent")),
		nodeprops.Num("Amount", "amount", 0,
			"Amount in the smallest currency unit (e.g. cents).", false).
			WithDisplayOptions(nodeprops.Show("operation", "createPaymentIntent")),
		nodeprops.Str("Currency", "currency", "usd",
			"Three-letter currency code (lowercase).",
			"usd", false).
			WithDisplayOptions(nodeprops.Show("operation", "createPaymentIntent")),
		nodeprops.Str("Customer ID", "customerId", "",
			"Existing customer the payment intent belongs to.",
			"cus_…", false).
			WithDisplayOptions(nodeprops.Show("operation", "createPaymentIntent")),
		nodeprops.Num("Limit", "limit", 0,
			"Maximum number of payments to list (0 = the API default).", false).
			WithDisplayOptions(nodeprops.Show("operation", "listPayments")),
	}
	return append(props, nodeprops.Settings()...)
}
