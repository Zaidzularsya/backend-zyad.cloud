package quotationpdf

import (
	"strconv"

	"zyad.cloud/internal/shared/docpdf"
)

// Renderer membungkus docpdf.Renderer untuk view-model penawaran.
type Renderer struct{ inner *docpdf.Renderer }

func NewRenderer() *Renderer { return &Renderer{inner: docpdf.NewRenderer()} }

func (r *Renderer) Render(doc Document) ([]byte, error) { return r.inner.Render(doc.PDF()) }

func itoa(n int) string { return strconv.Itoa(n) }
