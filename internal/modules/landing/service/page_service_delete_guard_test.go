package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
)

type openActivationFinderFake struct {
	open *landingdomain.LandingPageActivation
	err  error
}

func (f openActivationFinderFake) FindOpenByPage(context.Context, coretenant.Scope, string) (landingdomain.LandingPageActivation, error) {
	if f.err != nil {
		return landingdomain.LandingPageActivation{}, f.err
	}
	if f.open == nil {
		return landingdomain.LandingPageActivation{}, pgx.ErrNoRows
	}
	return *f.open, nil
}

func deleteGuardService(page landingdomain.LandingPage, finder ActivationOpenFinder) (PageService, *pageServicePageRepoStub) {
	repo := &pageServicePageRepoStub{findByIDPage: page}
	opts := []PageServiceOption{}
	if finder != nil {
		opts = append(opts, WithLandingPageActivationGuard(finder))
	}
	return NewPageService(repo, &pageServiceSectionRepoStub{}, opts...), repo
}

func inHours(h int) *time.Time {
	t := time.Now().UTC().Add(time.Duration(h) * time.Hour)
	return &t
}

func openActivation(dueInHours int) *landingdomain.LandingPageActivation {
	return &landingdomain.LandingPageActivation{
		Status: landingdomain.ActivationStatusOpen, InvoiceDueAt: *inHours(dueInHours),
	}
}

func requireDeleteBlocked(t *testing.T, err error, repo *pageServicePageRepoStub) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "PAGE_HAS_ACTIVE_ACTIVATION" || appErr.Status != http.StatusConflict {
		t.Fatalf("err = %v, want 409 PAGE_HAS_ACTIVE_ACTIVATION", err)
	}
	if repo.deleteCalled {
		t.Fatal("halaman tidak boleh terhapus saat guard menolak")
	}
}

func TestPageDeleteBlockedWhilePaidAndPublished(t *testing.T) {
	page := landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusPublished, ActiveUntil: inHours(24 * 30)}
	svc, repo := deleteGuardService(page, openActivationFinderFake{})
	requireDeleteBlocked(t, svc.Delete(context.Background(), mustLandingScope(t), "p1"), repo)
}

func TestPageDeleteBlockedWhileInvoiceAwaitingPayment(t *testing.T) {
	page := landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusDraft}
	svc, repo := deleteGuardService(page, openActivationFinderFake{open: openActivation(1)})
	requireDeleteBlocked(t, svc.Delete(context.Background(), mustLandingScope(t), "p1"), repo)
}

func TestPageDeleteAllowedWhenNothingIsSold(t *testing.T) {
	cases := map[string]struct {
		page landingdomain.LandingPage
		open *landingdomain.LandingPageActivation
	}{
		"halaman biasa tanpa aktivasi":    {landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusPublished}, nil},
		"masa aktif sudah habis":          {landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusPublished, ActiveUntil: inHours(-5)}, nil},
		"invoice sudah lewat jatuh tempo": {landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusDraft}, openActivation(-2)},
		"draft tanpa invoice":             {landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusDraft}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo := deleteGuardService(tc.page, openActivationFinderFake{open: tc.open})
			if err := svc.Delete(context.Background(), mustLandingScope(t), "p1"); err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			if !repo.deleteCalled {
				t.Fatal("halaman seharusnya terhapus")
			}
		})
	}
}

func TestPageDeleteFailsClosedWhenActivationLookupFails(t *testing.T) {
	page := landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusDraft}
	svc, repo := deleteGuardService(page, openActivationFinderFake{err: errors.New("db down")})
	if err := svc.Delete(context.Background(), mustLandingScope(t), "p1"); err == nil || repo.deleteCalled {
		t.Fatalf("err=%v deleteCalled=%v: gagal baca aktivasi tidak boleh melanjutkan penghapusan", err, repo.deleteCalled)
	}
}

func TestPageDeleteWithoutGuardKeepsLegacyBehavior(t *testing.T) {
	page := landingdomain.LandingPage{ID: "p1", Status: landingdomain.PageStatusPublished, ActiveUntil: inHours(48)}
	svc, repo := deleteGuardService(page, nil)
	if err := svc.Delete(context.Background(), mustLandingScope(t), "p1"); err != nil || !repo.deleteCalled {
		t.Fatalf("err=%v deleteCalled=%v", err, repo.deleteCalled)
	}
}
