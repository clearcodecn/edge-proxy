package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed template/*.html static/*
var assets embed.FS

// Templates holds one parsed *template.Template per page.
// Each page is parsed alongside layout.html so its {{define "content"}}
// block does not collide with other pages' content blocks.
type Templates struct {
	Login     *template.Template
	Domains   *template.Template
	Upstreams *template.Template
	Config    *template.Template
}

// MustLoadTemplates parses layout + each page template independently.
// Panics on parse error (called once at startup).
func MustLoadTemplates() *Templates {
	parse := func(page string) *template.Template {
		t, err := template.ParseFS(assets, "template/layout.html", "template/"+page)
		if err != nil {
			panic("parse template " + page + ": " + err.Error())
		}
		return t
	}
	return &Templates{
		Login:     parse("login.html"),
		Domains:   parse("domains.html"),
		Upstreams: parse("upstreams.html"),
		Config:    parse("config.html"),
	}
}

// StaticHandler serves files under the embedded static/ directory.
// Mount under /static/ via chi: r.Handle("/static/*", web.StaticHandler())
func StaticHandler() http.Handler {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic("static sub fs: " + err.Error())
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
