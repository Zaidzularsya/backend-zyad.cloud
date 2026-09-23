package domain

// OrganizationMember adalah ringkasan user anggota organization untuk
// kebutuhan pemilihan owner/assignee di CRM — sengaja minimal (tanpa role,
// status, dll) supaya endpoint lookup-nya aman dibuka ke pemegang lead.read.
type OrganizationMember struct {
	UserID string
	Name   string
	Email  string
}
