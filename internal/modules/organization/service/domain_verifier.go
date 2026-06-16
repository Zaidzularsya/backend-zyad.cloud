package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	coreauth "zyad.cloud/internal/core/auth"
)

var ErrDomainChallengeNotFound = errors.New("domain verification challenge not found")

type DomainVerifier interface {
	VerifyTXT(context.Context, string, string) error
}

type DNSDomainVerifier struct {
	resolver *net.Resolver
}

func NewDNSDomainVerifier(resolver *net.Resolver) *DNSDomainVerifier {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &DNSDomainVerifier{resolver: resolver}
}

func (v *DNSDomainVerifier) VerifyTXT(
	ctx context.Context,
	recordName string,
	expectedHash string,
) error {
	records, err := v.resolver.LookupTXT(ctx, strings.TrimSpace(recordName))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return ErrDomainChallengeNotFound
		}
		return fmt.Errorf("lookup domain verification TXT record: %w", err)
	}
	for _, record := range records {
		if coreauth.HashToken(strings.TrimSpace(record)) == expectedHash {
			return nil
		}
	}
	return ErrDomainChallengeNotFound
}
