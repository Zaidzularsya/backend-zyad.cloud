// Package whatsapp holds the WhatsApp provider adapters: the Client interface
// used by notification dispatchers, a NoopClient, and the WAHA (WhatsApp HTTP
// API, self-hosted) HTTP client plus webhook signature verification.
//
// Business rules (sessions per tenant, conversations, matching to CRM) live in
// internal/modules/whatsapp, not here.
package whatsapp
