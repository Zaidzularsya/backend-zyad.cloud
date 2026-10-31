package service

import (
	"context"
	"errors"
	"fmt"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

const (
	pppActivePath  = "/ppp/active"
	pppProfilePath = "/ppp/profile"
	rowIDKey       = ".id"
)

// Executor performs the router-side steps of isolate/restore. It never touches
// the database: the caller flips DB state only after a nil return.
type Executor struct {
	routers *RouterService
}

func NewExecutor(routers *RouterService) *Executor { return &Executor{routers: routers} }

// Isolate moves the secret to the router's isolir profile, drops its active
// sessions and confirms the result. It returns the profile to restore to:
// the router's current profile when it is not the isolir profile, otherwise
// secret.OriginalProfile. Idempotent when already isolated.
func (e *Executor) Isolate(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) (string, error) {
	router, client, secrets, err := e.routers.openClient(ctx, scope, secret.RouterID)
	if err != nil {
		return "", err
	}
	// Re-checked at execution time; an earlier check may be stale.
	if !router.IsolirReady || router.IsolirProfileName == "" {
		return "", mikrotik.ErrRouterNotIsolirReady
	}
	isolir := router.IsolirProfileName

	id, current, err := findSecret(ctx, client, secret.Name, secrets)
	if err != nil {
		return "", err
	}
	original := current
	if current == isolir {
		original = secret.OriginalProfile
	}
	if original == "" || original == isolir {
		return "", mikrotik.ErrRestoreTargetInvalid
	}
	if current != isolir {
		if err := client.Set(ctx, secretPath, id, pmk.Row{"profile": isolir}); err != nil {
			return "", translateClientError(err, secrets...)
		}
	}
	// From here the router is on the isolir profile (Set acknowledged, or it
	// already was): a later failure must tell the caller so the DB can follow.
	if err := dropActiveSessions(ctx, client, secret.Name, secrets); err != nil {
		return "", &ProfileChangedError{Original: original, Err: err}
	}
	_, got, err := findSecret(ctx, client, secret.Name, secrets)
	if err != nil {
		return "", &ProfileChangedError{Original: original, Err: err}
	}
	if got != isolir {
		// Set was acknowledged but not applied: the router is not known to have
		// changed, so this is a plain transient failure.
		return "", fmt.Errorf("%w: profile not applied on router", mikrotik.ErrUnreachable)
	}
	return original, nil
}

// ProfileChangedError reports an isolate that failed after the router was
// already on the isolir profile. Original is the profile to restore to.
// Unwrap keeps errors.Is/As and Classify working on the cause.
type ProfileChangedError struct {
	Original string
	Err      error
}

func (e *ProfileChangedError) Error() string {
	return "isolir profile applied on router but a later step failed"
}

func (e *ProfileChangedError) Unwrap() error { return e.Err }

// Restore moves the secret back to secret.OriginalProfile. It does not require
// isolir_ready (restoring must keep working after setup is cleaned up) but the
// target must exist on the router and must not be the isolir profile.
func (e *Executor) Restore(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) error {
	router, client, secrets, err := e.routers.openClient(ctx, scope, secret.RouterID)
	if err != nil {
		return err
	}
	target := secret.OriginalProfile
	if target == "" || (router.IsolirProfileName != "" && target == router.IsolirProfileName) {
		return mikrotik.ErrRestoreTargetInvalid
	}
	profiles, err := client.Print(ctx, pppProfilePath, pmk.Row{"name": target})
	if err != nil {
		return translateClientError(err, secrets...)
	}
	if len(profiles) == 0 {
		return mikrotik.ErrRestoreProfileMissing
	}
	id, current, err := findSecret(ctx, client, secret.Name, secrets)
	if err != nil {
		return err
	}
	if current != target {
		if err := client.Set(ctx, secretPath, id, pmk.Row{"profile": target}); err != nil {
			return translateClientError(err, secrets...)
		}
	}
	// Sessions are dropped so the customer reconnects on the restored profile.
	if err := dropActiveSessions(ctx, client, secret.Name, secrets); err != nil {
		return err
	}
	return confirmProfile(ctx, client, secret.Name, target, secrets)
}

func findSecret(ctx context.Context, client pmk.RouterClient, name string, secrets []string) (id, profile string, err error) {
	rows, err := client.Print(ctx, secretPath, pmk.Row{"name": name})
	if err != nil {
		return "", "", translateClientError(err, secrets...)
	}
	for _, r := range rows {
		// Defensive: do not trust server-side filtering.
		if r["name"] == name {
			return r[rowIDKey], r["profile"], nil
		}
	}
	return "", "", mikrotik.ErrSecretMissingOnRouter
}

func dropActiveSessions(ctx context.Context, client pmk.RouterClient, name string, secrets []string) error {
	rows, err := client.Print(ctx, pppActivePath, pmk.Row{"name": name})
	if err != nil {
		return translateClientError(err, secrets...)
	}
	for _, r := range rows {
		if r["name"] != name {
			continue
		}
		// A session that vanished on its own is the desired outcome.
		if err := client.Remove(ctx, pppActivePath, r[rowIDKey]); err != nil && !errors.Is(err, pmk.ErrNotFound) {
			return translateClientError(err, secrets...)
		}
	}
	return nil
}

// confirmProfile re-reads the secret; a mismatch is transient so the task
// retries instead of the DB claiming a state the router does not have.
func confirmProfile(ctx context.Context, client pmk.RouterClient, name, want string, secrets []string) error {
	_, got, err := findSecret(ctx, client, name, secrets)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%w: profile not applied on router", mikrotik.ErrUnreachable)
	}
	return nil
}
