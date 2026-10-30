package service

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
)

// TestCheckStatusesListsEveryConstant parses the package sources so that a new
// CheckStatus* constant missing from CheckStatuses (and therefore from the
// integration test that stores every status) fails here.
func TestCheckStatusesListsEveryConstant(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						if !strings.HasPrefix(name.Name, "CheckStatus") || i >= len(vs.Values) {
							continue
						}
						lit, ok := vs.Values[i].(*ast.BasicLit)
						if !ok {
							t.Fatalf("%s is not a string literal", name.Name)
						}
						v, _ := strconv.Unquote(lit.Value)
						found = append(found, v)
					}
				}
			}
		}
	}
	listed := append([]string(nil), CheckStatuses...)
	sort.Strings(found)
	sort.Strings(listed)
	if len(found) == 0 || strings.Join(found, ",") != strings.Join(listed, ",") {
		t.Fatalf("CheckStatus* constants = %v, CheckStatuses = %v", found, listed)
	}
}

func TestCheckOnNoLongerAllowedHostRecordsHostNotAllowed(t *testing.T) {
	svc, repo, factory := newTestService(t)
	r := registerOne(t, svc)
	// The operator narrowed the allow list after registration.
	svc.allowedPrivate = []netip.Prefix{netip.MustParsePrefix("10.9.0.0/24")}
	got, err := svc.Check(context.Background(), testScope(t), r.ID)
	if !errors.Is(err, mikrotik.ErrHostNotAllowed) {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
	if got.LastCheckStatus != CheckStatusHostBlocked || repo.checks[len(repo.checks)-1] != CheckStatusHostBlocked {
		t.Fatalf("recorded status = %q (checks %v), want %q", got.LastCheckStatus, repo.checks, CheckStatusHostBlocked)
	}
	if factory.detectCalls != 1 {
		t.Fatalf("detect calls = %d, the blocked host must not be probed by Check", factory.detectCalls)
	}
}
