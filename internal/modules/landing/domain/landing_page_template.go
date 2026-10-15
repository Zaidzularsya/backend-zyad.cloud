package domain

import "time"

type PageTemplateStatus string

const (
	PageTemplateStatusDraft     PageTemplateStatus = "draft"
	PageTemplateStatusPublished PageTemplateStatus = "published"
	PageTemplateStatusArchived  PageTemplateStatus = "archived"
)

func (s PageTemplateStatus) IsValid() bool {
	switch s {
	case PageTemplateStatusDraft, PageTemplateStatusPublished, PageTemplateStatusArchived:
		return true
	}
	return false
}

type PageTemplateSource string

const (
	PageTemplateSourceSeed  PageTemplateSource = "seed"
	PageTemplateSourceAdmin PageTemplateSource = "admin"
)

// PageTemplate adalah template halaman GrapesJS global (lintas organisasi).
// HTML dan CSS kosong pada hasil List.
type PageTemplate struct {
	ID, Slug, Name, Description, Category string
	SortOrder                             int
	Status                                PageTemplateStatus
	PlatformOnly                          bool
	HTML, CSS                             string
	Source                                PageTemplateSource
	SeedVersion                           *int
	SeedModified                          bool
	SourcePageID                          *string
	CreatedBy, UpdatedBy                  string
	CreatedAt, UpdatedAt                  time.Time
}

var PageTemplateCategories = []string{
	"dasar", "bisnis-jasa", "personal", "acara", "kesehatan", "pendidikan",
	"komunitas", "kuliner", "properti", "ritel-umkm", "teknologi-isp", "platform",
}

func IsValidPageTemplateCategory(c string) bool {
	for _, v := range PageTemplateCategories {
		if v == c {
			return true
		}
	}
	return false
}
