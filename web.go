package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/rebrudiy/beldex-go/walletrpc"
)

//go:embed web/*.html
var webFS embed.FS

var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"bdx": walletrpc.FormatBDX,
	"pct": func(raised, goal uint64) int {
		if goal == 0 {
			return 0
		}
		return int(min(100, raised*100/goal))
	},
	"ago": func(t time.Time) string { return time.Since(t).Round(time.Second).String() },
}).ParseFS(webFS, "web/*.html"))

// Audit is published on every page: with these two strings anyone can restore
// a view-only wallet and recompute every total shown here.
type Audit struct {
	PrimaryAddress string `json:"primary_address"`
	ViewKey        string `json:"view_key"`
	Network        string `json:"network"`
}

type Server struct {
	store   *Store
	tracker *Tracker
	audit   Audit
}

type campaignView struct {
	Campaign
	Progress
	Updated time.Time
	Audit   Audit
}

func (s *Server) view(c Campaign) campaignView {
	p, updated := s.tracker.Progress(c.SubIndex)
	return campaignView{Campaign: c, Progress: p, Updated: updated, Audit: s.audit}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /c/{id}", s.campaign)
	mux.HandleFunc("GET /api/campaigns", s.apiList)
	mux.HandleFunc("GET /api/campaigns/{id}", s.apiCampaign)
	return mux
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	var views []campaignView
	for _, c := range s.store.List() {
		views = append(views, s.view(c))
	}
	render(w, "index.html", map[string]any{"Campaigns": views, "Audit": s.audit})
}

func (s *Server) campaign(w http.ResponseWriter, r *http.Request) {
	c, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	render(w, "campaign.html", s.view(c))
}

func (s *Server) apiList(w http.ResponseWriter, r *http.Request) {
	var views []campaignView
	for _, c := range s.store.List() {
		views = append(views, s.view(c))
	}
	writeJSON(w, views)
}

func (s *Server) apiCampaign(w http.ResponseWriter, r *http.Request) {
	c, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, s.view(c))
}

func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
