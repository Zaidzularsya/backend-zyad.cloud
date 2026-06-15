package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTenantRepositoryInterfacesRequireScope(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	files := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		node, err := parser.ParseFile(
			files,
			filepath.Clean(entry.Name()),
			nil,
			parser.SkipObjectResolution,
		)
		if err != nil {
			t.Fatalf("ParseFile(%s) error = %v", entry.Name(), err)
		}
		assertTenantRepositoryDeclarations(t, node)
	}
}

func assertTenantRepositoryDeclarations(t *testing.T, file *ast.File) {
	t.Helper()
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			assertTenantRepositoryMethod(t, function)
			continue
		}
		typeDeclaration, ok := declaration.(*ast.GenDecl)
		if !ok || typeDeclaration.Tok != token.TYPE {
			continue
		}
		for _, spec := range typeDeclaration.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(typeSpec.Name.Name, "Repository") ||
				strings.HasPrefix(typeSpec.Name.Name, "Platform") {
				continue
			}
			repositoryInterface, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			for _, method := range repositoryInterface.Methods.List {
				function, ok := method.Type.(*ast.FuncType)
				if !ok {
					continue
				}
				if !hasTenantScope(function.Params) {
					t.Errorf(
						"%s.%s must accept tenant.Scope",
						typeSpec.Name.Name,
						methodName(method),
					)
				}
				if hasFreeOrganizationID(function.Params) {
					t.Errorf(
						"%s.%s must not accept free organizationID string",
						typeSpec.Name.Name,
						methodName(method),
					)
				}
			}
		}
	}
}

func assertTenantRepositoryMethod(t *testing.T, function *ast.FuncDecl) {
	t.Helper()
	receiverName := repositoryReceiverName(function.Recv)
	if receiverName == "" ||
		!strings.HasSuffix(receiverName, "Repository") ||
		strings.HasPrefix(receiverName, "Platform") ||
		strings.HasPrefix(function.Name.Name, "New") {
		return
	}
	if !hasTenantScope(function.Type.Params) {
		t.Errorf("%s.%s must accept tenant.Scope", receiverName, function.Name.Name)
	}
	if hasFreeOrganizationID(function.Type.Params) {
		t.Errorf(
			"%s.%s must not accept free organizationID string",
			receiverName,
			function.Name.Name,
		)
	}
}

func repositoryReceiverName(receivers *ast.FieldList) string {
	if receivers == nil || len(receivers.List) != 1 {
		return ""
	}
	receiverType := receivers.List[0].Type
	if pointer, ok := receiverType.(*ast.StarExpr); ok {
		receiverType = pointer.X
	}
	identifier, ok := receiverType.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func hasTenantScope(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		selector, ok := field.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Scope" {
			continue
		}
		packageName, ok := selector.X.(*ast.Ident)
		if ok && packageName.Name == "coretenant" {
			return true
		}
	}
	return false
}

func hasFreeOrganizationID(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		parameterType, ok := field.Type.(*ast.Ident)
		if !ok || parameterType.Name != "string" {
			continue
		}
		for _, name := range field.Names {
			normalized := strings.ToLower(name.Name)
			if normalized == "organizationid" || normalized == "organization_id" {
				return true
			}
		}
	}
	return false
}

func methodName(field *ast.Field) string {
	if len(field.Names) == 0 {
		return "<embedded>"
	}
	return field.Names[0].Name
}
