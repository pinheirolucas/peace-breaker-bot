package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/pinheirolucas/peace-breaker-bot/pkg/i18n"
	"github.com/pinheirolucas/peace-breaker-bot/pkg/instant"
)

type openAPIDoc struct {
	OpenAPI string                            `yaml:"openapi"`
	Paths   map[string]map[string]interface{} `yaml:"paths"`
}

func loadOpenAPIDoc(t *testing.T) openAPIDoc {
	t.Helper()

	var doc openAPIDoc
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("openapi.yaml does not parse: %v", err)
	}

	return doc
}

func TestOpenAPISpecIsValidYAML(t *testing.T) {
	if doc := loadOpenAPIDoc(t); doc.OpenAPI == "" {
		t.Error("openapi.yaml is missing the top-level openapi version field")
	}
}

func TestOpenAPISpecDocumentsEveryRoute(t *testing.T) {
	doc := loadOpenAPIDoc(t)

	for _, rt := range New(instant.NewPlayer(), connectedBot()).routes() {
		method, path, ok := strings.Cut(rt.pattern, " ")
		if !ok {
			t.Errorf("route %q has no method", rt.pattern)
			continue
		}

		if _, ok := doc.Paths[path][strings.ToLower(method)]; !ok {
			t.Errorf("openapi.yaml has no %s %s; document it in pkg/server/v1/openapi.yaml", method, path)
		}
	}
}

func TestOpenAPISpecHasNoUnservedRoutes(t *testing.T) {
	served := map[string]bool{}
	for _, rt := range New(instant.NewPlayer(), connectedBot()).routes() {
		served[rt.pattern] = true
	}

	for path, ops := range loadOpenAPIDoc(t).Paths {
		for method := range ops {
			if method == "parameters" {
				continue
			}
			if pattern := strings.ToUpper(method) + " " + path; !served[pattern] {
				t.Errorf("openapi.yaml documents %s, which server.go doesn't serve", pattern)
			}
		}
	}
}

func errorLabels(t *testing.T) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	seen := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); !ok || ident.Name != "writeErrorMessage" {
			return true
		}
		if lit, ok := call.Args[len(call.Args)-1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			seen[strings.Trim(lit.Value, `"`)] = true
		}
		return true
	})

	labels := make([]string, 0, len(seen))
	for label := range seen {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	if len(labels) == 0 {
		t.Fatal("found no writeErrorMessage labels in server.go")
	}

	return labels
}

func TestEveryErrorLabelHasCatalogText(t *testing.T) {
	for _, label := range errorLabels(t) {
		for _, lang := range i18n.Supported {
			if i18n.Text(lang, label) == label {
				t.Errorf("label %q has no %s text in pkg/i18n", label, lang)
			}
		}
	}
}

func TestEveryErrorLabelIsDocumented(t *testing.T) {
	spec := string(openAPISpec)
	for _, label := range errorLabels(t) {
		if !strings.Contains(spec, "label: "+label) {
			t.Errorf("label %q has no example in openapi.yaml", label)
		}
	}
}

func TestHandleOpenAPISpecServesTheEmbeddedDocument(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleOpenAPISpec(rec, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", got)
	}
	if rec.Body.Len() == 0 {
		t.Error("response body is empty")
	}
}

func TestHandleDocsServesAnHTMLPageReferencingTheSpec(t *testing.T) {
	s := New(instant.NewPlayer(), connectedBot())

	rec := httptest.NewRecorder()
	s.handleDocs(rec, httptest.NewRequest(http.MethodGet, "/api/docs", nil))

	if got := rec.Header().Get("Content-Type"); got != "text/html" {
		t.Errorf("Content-Type = %q, want text/html", got)
	}
	if !strings.Contains(rec.Body.String(), "/api/v1/openapi.yaml") {
		t.Error("docs page does not reference /api/v1/openapi.yaml")
	}
}
