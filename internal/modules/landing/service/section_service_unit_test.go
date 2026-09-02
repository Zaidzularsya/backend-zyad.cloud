package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type sectionServiceRepoStub struct {
	listByPageItems  []landingdomain.LandingSection
	listByPageErr    error
	createCalled     bool
	updateCalled     bool
	findByIDResult   landingdomain.LandingSection
	findByIDErr      error
	replaceAllCalled bool
	replaceAllParams repository.ReplaceAllParams
	replaceAllResult []landingdomain.LandingSection
	replaceAllErr    error
}

func (s *sectionServiceRepoStub) Create(
	context.Context,
	coretenant.Scope,
	repository.CreateSectionParams,
) (landingdomain.LandingSection, error) {
	s.createCalled = true
	return landingdomain.LandingSection{
		ID:            "section-1",
		LandingPageID: "page-1",
	}, nil
}

func (s *sectionServiceRepoStub) FindByID(
	context.Context,
	coretenant.Scope,
	string,
) (landingdomain.LandingSection, error) {
	return s.findByIDResult, s.findByIDErr
}

func (s *sectionServiceRepoStub) ListByPage(
	context.Context,
	coretenant.Scope,
	string,
) ([]landingdomain.LandingSection, error) {
	return s.listByPageItems, s.listByPageErr
}

func (s *sectionServiceRepoStub) Update(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateSectionParams,
) (landingdomain.LandingSection, error) {
	s.updateCalled = true
	return landingdomain.LandingSection{}, nil
}

func (s *sectionServiceRepoStub) Reorder(
	context.Context,
	coretenant.Scope,
	string,
	[]repository.SectionReorderParam,
) error {
	return nil
}

func (s *sectionServiceRepoStub) ReplaceAll(
	_ context.Context,
	_ coretenant.Scope,
	params repository.ReplaceAllParams,
) ([]landingdomain.LandingSection, error) {
	s.replaceAllCalled = true
	s.replaceAllParams = params
	return s.replaceAllResult, s.replaceAllErr
}

func (s *sectionServiceRepoStub) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

type sectionServiceQuotaGuardStub struct {
	organizationID string
	featureKey     string
	limitKey       string
	usedValue      int64
	delta          int64
	err            error
}

func (s *sectionServiceQuotaGuardStub) RequireQuotaValue(
	_ context.Context,
	organizationID string,
	featureKey string,
	limitKey string,
	usedValue int64,
	delta int64,
) error {
	s.organizationID = organizationID
	s.featureKey = featureKey
	s.limitKey = limitKey
	s.usedValue = usedValue
	s.delta = delta
	return s.err
}

func TestSectionServiceCreateUsesBillingQuotaGuard(t *testing.T) {
	repo := &sectionServiceRepoStub{
		listByPageItems: []landingdomain.LandingSection{
			{ID: "section-1"},
			{ID: "section-2"},
		},
	}
	guard := &sectionServiceQuotaGuardStub{}
	service := NewSectionService(repo, WithLandingSectionQuotaGuard(guard))

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "hero-3",
		Type:          landingdomain.SectionTypeHero,
		Name:          "Hero",
		IsEnabled:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist section when quota allows")
	}
	if guard.organizationID != landingServiceOrganizationID ||
		guard.featureKey != landingdomain.FeatureLandingMaxSections ||
		guard.limitKey != "limit" ||
		guard.usedValue != 2 ||
		guard.delta != 1 {
		t.Fatalf("guard = %#v", guard)
	}
}

func TestSectionServiceCreateStopsWhenQuotaExceeded(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	guardErr := errors.New("quota exceeded")
	service := NewSectionService(
		repo,
		WithLandingSectionQuotaGuard(&sectionServiceQuotaGuardStub{err: guardErr}),
	)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "hero-1",
		Type:          landingdomain.SectionTypeHero,
		Name:          "Hero",
		IsEnabled:     true,
	})
	if !errors.Is(err, guardErr) {
		t.Fatalf("Create() error = %v, want %v", err, guardErr)
	}
	if repo.createCalled {
		t.Fatal("Create() should not persist section when quota guard fails")
	}
}

func TestSectionServiceCreateRejectsPlatformCatalogForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "pricing-1",
		Type:          landingdomain.SectionTypePricing,
		Name:          "Pricing",
		IsEnabled:     true,
		Content:       map[string]any{"source": "platform_catalog"},
	})
	if err == nil {
		t.Fatal("Create() expected error for non-platform organization using platform_catalog source")
	}
	if repo.createCalled {
		t.Fatal("Create() should not persist section when pricing source is not allowed")
	}
}

func TestSectionServiceCreateAllowsPlatformCatalogForPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypePlatform, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "pricing-1",
		Type:          landingdomain.SectionTypePricing,
		Name:          "Pricing",
		IsEnabled:     true,
		Content:       map[string]any{"source": "platform_catalog"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist section when platform organization uses platform_catalog source")
	}
}

func TestSectionServiceCreateAllowsCustomPricingForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "pricing-1",
		Type:          landingdomain.SectionTypePricing,
		Name:          "Pricing",
		IsEnabled:     true,
		Content:       map[string]any{"source": "custom"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist section when using custom pricing source")
	}
}

func TestSectionServiceUpdateRejectsPlatformCatalogForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{
		findByIDResult: landingdomain.LandingSection{
			ID:   "section-1",
			Type: landingdomain.SectionTypePricing,
		},
	}
	service := NewSectionService(repo)

	_, err := service.Update(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "section-1", repository.UpdateSectionParams{
		Content: map[string]any{"source": "platform_catalog"},
	})
	if err == nil {
		t.Fatal("Update() expected error for non-platform organization using platform_catalog source")
	}
	if repo.updateCalled {
		t.Fatal("Update() should not persist section when pricing source is not allowed")
	}
}

func TestSectionServiceUpdateAllowsPlatformCatalogForPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{
		findByIDResult: landingdomain.LandingSection{
			ID:   "section-1",
			Type: landingdomain.SectionTypePricing,
		},
	}
	service := NewSectionService(repo)

	_, err := service.Update(context.Background(), mustLandingScope(t), coretenant.OrganizationTypePlatform, "section-1", repository.UpdateSectionParams{
		Content: map[string]any{"source": "platform_catalog"},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error = %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("Update() should persist section when platform organization uses platform_catalog source")
	}
}

func TestSectionServiceCreateRejectsUnknownFooterVariant(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "footer-1",
		Type:          landingdomain.SectionTypeFooter,
		Name:          "Footer",
		IsEnabled:     true,
		Style:         map[string]any{"variant": "carousel"},
	})
	if err == nil {
		t.Fatal("Create() expected error for unknown footer variant")
	}
	if repo.createCalled {
		t.Fatal("Create() should not persist section when footer variant is not allowed")
	}
}

func TestSectionServiceCreateAllowsKnownFooterVariants(t *testing.T) {
	for _, variant := range []string{"", "default", "simple", "newsletter", "mega"} {
		repo := &sectionServiceRepoStub{}
		service := NewSectionService(repo)

		_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
			LandingPageID: "page-1",
			Key:           "footer-1",
			Type:          landingdomain.SectionTypeFooter,
			Name:          "Footer",
			IsEnabled:     true,
			Style:         map[string]any{"variant": variant},
		})
		if err != nil {
			t.Fatalf("Create() unexpected error for variant %q = %v", variant, err)
		}
		if !repo.createCalled {
			t.Fatalf("Create() should persist section for variant %q", variant)
		}
	}
}

func TestSectionServiceCreateIgnoresVariantForNonFooterSection(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "hero-1",
		Type:          landingdomain.SectionTypeHero,
		Name:          "Hero",
		IsEnabled:     true,
		Style:         map[string]any{"variant": "anything-goes"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist non-footer section regardless of style.variant")
	}
}

func TestSectionServiceUpdateRejectsUnknownFooterVariant(t *testing.T) {
	repo := &sectionServiceRepoStub{
		findByIDResult: landingdomain.LandingSection{
			ID:   "section-1",
			Type: landingdomain.SectionTypeFooter,
		},
	}
	service := NewSectionService(repo)

	_, err := service.Update(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "section-1", repository.UpdateSectionParams{
		Style: map[string]any{"variant": "carousel"},
	})
	if err == nil {
		t.Fatal("Update() expected error for unknown footer variant")
	}
	if repo.updateCalled {
		t.Fatal("Update() should not persist section when footer variant is not allowed")
	}
}

func TestSectionServiceSanitizeStringStripsXSSVectors(t *testing.T) {
	svc := &sectionService{}

	cases := map[string]string{
		"Welcome <script>alert(1)</script>":            "Welcome ",
		"Click <a href='javascript:alert(1)'>here</a>": "Click here",
		"<img src=x onerror=alert(1)>":                 "",
		"<svg onload=alert(1)>":                        "",
		"<iframe src='bad.com'></iframe> nested":       " nested",
		`Click <a href="https://example.com">here</a>`: `Click <a href="https://example.com" rel="nofollow">here</a>`,
		"plain text with no tags":                      "plain text with no tags",
	}

	for in, want := range cases {
		if got := svc.sanitizeString(in); got != want {
			t.Errorf("sanitizeString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSectionServiceSanitizeMapRecursesThroughNestedStructures(t *testing.T) {
	svc := &sectionService{}

	content := map[string]any{
		"title": "Welcome <script>alert(1)</script>",
		"nested": []any{
			map[string]any{"text": "<iframe src='bad.com'></iframe> nested"},
		},
		"count": 5,
	}

	sanitized := svc.sanitizeMap(content)

	if sanitized["title"] != "Welcome " {
		t.Errorf("sanitizeMap() title = %v", sanitized["title"])
	}
	nestedArr, ok := sanitized["nested"].([]any)
	if !ok || len(nestedArr) != 1 {
		t.Fatalf("sanitizeMap() nested = %#v", sanitized["nested"])
	}
	nestedMap, ok := nestedArr[0].(map[string]any)
	if !ok || nestedMap["text"] != " nested" {
		t.Fatalf("sanitizeMap() nested text = %#v", nestedArr[0])
	}
	if sanitized["count"] != 5 {
		t.Errorf("sanitizeMap() should leave non-string values untouched, got %v", sanitized["count"])
	}
}

func TestSectionServiceReplaceAllSanitizesAndChecksQuota(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	guard := &sectionServiceQuotaGuardStub{}
	service := NewSectionService(repo, WithLandingSectionQuotaGuard(guard))

	items := []repository.ReplaceSectionItem{
		{
			Key:  "hero-1",
			Type: landingdomain.SectionTypeHero,
			Name: "Hero",
			Content: map[string]any{
				"title": "Welcome <script>alert(1)</script>",
			},
			Style: map[string]any{
				"variant":    "default",
				"onload":     "alert(1)",
				"background": map[string]any{"image": "javascript:alert(1)", "color": "#fff"},
			},
		},
		{
			ID:   "11111111-1111-1111-1111-111111111111",
			Key:  "cta-1",
			Type: landingdomain.SectionTypeCTA,
			Name: "CTA",
		},
	}

	_, err := service.ReplaceAll(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "page-1", items, "actor-1")
	if err != nil {
		t.Fatalf("ReplaceAll() error = %v", err)
	}
	if !repo.replaceAllCalled {
		t.Fatal("ReplaceAll() should reach the repository when validation passes")
	}
	if repo.replaceAllParams.LandingPageID != "page-1" || repo.replaceAllParams.ActorID != "actor-1" {
		t.Fatalf("ReplaceAll() params = %#v", repo.replaceAllParams)
	}

	if guard.featureKey != landingdomain.FeatureLandingMaxSections ||
		guard.limitKey != "limit" ||
		guard.usedValue != 2 ||
		guard.delta != 0 {
		t.Fatalf("quota guard = %#v", guard)
	}

	got := repo.replaceAllParams.Items[0]
	if got.Content["title"] != "Welcome " {
		t.Errorf("content not sanitized: %#v", got.Content["title"])
	}
	if _, exists := got.Style["onload"]; exists {
		t.Errorf("unknown style key should be dropped: %#v", got.Style)
	}
	if got.Style["variant"] != "default" {
		t.Errorf("known style key should survive: %#v", got.Style["variant"])
	}
	bg, ok := got.Style["background"].(map[string]any)
	if !ok {
		t.Fatalf("background style = %#v", got.Style["background"])
	}
	if bg["image"] != "" {
		t.Errorf("unsafe style url should be blanked: %#v", bg["image"])
	}
	if bg["color"] != "#fff" {
		t.Errorf("safe nested style value should survive: %#v", bg["color"])
	}
}

func TestSectionServiceReplaceAllRejectsUnknownFooterVariant(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.ReplaceAll(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "page-1", []repository.ReplaceSectionItem{
		{
			Key:   "footer-1",
			Type:  landingdomain.SectionTypeFooter,
			Name:  "Footer",
			Style: map[string]any{"variant": "carousel"},
		},
	}, "actor-1")
	if err == nil {
		t.Fatal("ReplaceAll() expected error for unknown footer variant")
	}
	if repo.replaceAllCalled {
		t.Fatal("ReplaceAll() should not reach the repository when an item is invalid")
	}
}

func TestSectionServiceReplaceAllRejectsPlatformCatalogForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.ReplaceAll(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "page-1", []repository.ReplaceSectionItem{
		{
			Key:     "pricing-1",
			Type:    landingdomain.SectionTypePricing,
			Name:    "Pricing",
			Content: map[string]any{"source": "platform_catalog"},
		},
	}, "actor-1")
	if err == nil {
		t.Fatal("ReplaceAll() expected error for non-platform org using platform_catalog source")
	}
	if repo.replaceAllCalled {
		t.Fatal("ReplaceAll() should not reach the repository when an item is invalid")
	}
}

func TestSectionServiceReplaceAllStopsWhenQuotaExceeded(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	guardErr := errors.New("quota exceeded")
	service := NewSectionService(repo, WithLandingSectionQuotaGuard(&sectionServiceQuotaGuardStub{err: guardErr}))

	_, err := service.ReplaceAll(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "page-1", []repository.ReplaceSectionItem{
		{Key: "hero-1", Type: landingdomain.SectionTypeHero, Name: "Hero"},
	}, "actor-1")
	if !errors.Is(err, guardErr) {
		t.Fatalf("ReplaceAll() error = %v, want %v", err, guardErr)
	}
	if repo.replaceAllCalled {
		t.Fatal("ReplaceAll() should not reach the repository when the quota guard fails")
	}
}

func TestSectionServiceSanitizeStyleMapDropsUnknownKeysAndUnsafeURLs(t *testing.T) {
	svc := &sectionService{}

	in := map[string]any{
		"variant":            "software_command",
		"renderer_component": "HeroSection",
		"align":              "center",
		"spacing":            map[string]any{"top": 40, "bottom": 24},
		"hero":               map[string]any{"backgroundImage": "https://cdn.example.com/a.png", "overlay": "dark"},
		"background":         map[string]any{"image": "javascript:alert(1)"},
		"evil":               "<script>alert(1)</script>",
		"onmouseover":        "x",
	}

	out := svc.sanitizeStyleMap(in)

	for _, dropped := range []string{"evil", "onmouseover"} {
		if _, exists := out[dropped]; exists {
			t.Errorf("key %q should be dropped, got %#v", dropped, out)
		}
	}
	if out["variant"] != "software_command" || out["align"] != "center" {
		t.Errorf("known scalar style keys mangled: %#v", out)
	}
	spacing := out["spacing"].(map[string]any)
	if spacing["top"] != 40 || spacing["bottom"] != 24 {
		t.Errorf("numeric nested style values should pass through: %#v", spacing)
	}
	hero := out["hero"].(map[string]any)
	if hero["backgroundImage"] != "https://cdn.example.com/a.png" {
		t.Errorf("safe https url should survive: %#v", hero["backgroundImage"])
	}
	bg := out["background"].(map[string]any)
	if bg["image"] != "" {
		t.Errorf("javascript: url should be blanked: %#v", bg["image"])
	}
}

func TestSanitizeStyleURL(t *testing.T) {
	cases := map[string]string{
		"https://cdn.example.com/a.png?v=2": "https://cdn.example.com/a.png?v=2",
		"http://example.com/b.jpg":          "http://example.com/b.jpg",
		"/media/local/c.webp":               "/media/local/c.webp",
		"javascript:alert(1)":               "",
		"data:image/png;base64,AAAA":        "",
		"//evil.com/x.png":                  "",
		`https://x/a.png") ; x:(`:           "",
		"  https://x/a.png  ":               "https://x/a.png",
		"":                                  "",
	}
	for in, want := range cases {
		if got := sanitizeStyleURL(in); got != want {
			t.Errorf("sanitizeStyleURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSectionServiceUpdateAllowsKnownFooterVariant(t *testing.T) {
	repo := &sectionServiceRepoStub{
		findByIDResult: landingdomain.LandingSection{
			ID:   "section-1",
			Type: landingdomain.SectionTypeFooter,
		},
	}
	service := NewSectionService(repo)

	_, err := service.Update(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "section-1", repository.UpdateSectionParams{
		Style: map[string]any{"variant": "mega"},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error = %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("Update() should persist section when footer variant is known")
	}
}
