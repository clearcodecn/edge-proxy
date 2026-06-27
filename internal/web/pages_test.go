package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderDomains_UsesScrollableMainContent(t *testing.T) {
	templates := MustLoadTemplates()
	pages := NewPageRenderer(templates, "node-a", "dev", "admin")

	rec := httptest.NewRecorder()
	pages.RenderDomains(rec, DomainListView{})

	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `class="flex-1 min-h-0 w-full min-w-0 overflow-y-auto p-4 lg:p-6"`) {
		t.Fatalf("body missing scrollable main content wrapper:\n%s", body)
	}
}

func TestRenderDomains_DoesNotForceModalOpenByDefault(t *testing.T) {
	templates := MustLoadTemplates()
	pages := NewPageRenderer(templates, "node-a", "dev", "admin")

	rec := httptest.NewRecorder()
	pages.RenderDomains(rec, DomainListView{})

	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, `class="modal modal-open"`) {
		t.Fatalf("body should not contain statically open modal markup:\n%s", body)
	}
}
