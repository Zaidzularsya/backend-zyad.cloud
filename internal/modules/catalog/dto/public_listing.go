package dto

type PublicCatalogResponse struct {
	Categories []PublicCategory `json:"categories"`
}

type PublicCategory struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Position int             `json:"position"`
	Listings []PublicListing `json:"listings"`
}

type PublicListing struct {
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Order       int             `json:"order"`
	Variants    []PublicVariant `json:"variants"`
	Benefits    []PublicBenefit `json:"benefits"`
}

type PublicVariant struct {
	ProductID        string `json:"product_id"`
	SKU              string `json:"sku"`
	ChargeType       string `json:"charge_type"`
	BillingFrequency string `json:"billing_frequency"`
	PaymentTiming    string `json:"payment_timing"`
	Currency         string `json:"currency"`
	BasePrice        string `json:"base_price"`
	TaxPercent       string `json:"tax_percent"`
	PriceWithTax     string `json:"price_with_tax"`
	CheckoutEnabled  bool   `json:"checkout_enabled"`
}

type PublicBenefit struct {
	Label string `json:"label"`
}
