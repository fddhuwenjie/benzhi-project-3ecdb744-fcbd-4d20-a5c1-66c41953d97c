package web

import (
	"embed"
	"io/fs"
	"net/http"

	"shelter-drill-gate/internal/workflow"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	workflow *workflow.Service
	mux      *http.ServeMux
}

func New(service *workflow.Service) *Server {
	server := &Server{workflow: service, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return securityHeaders(s.mux)
}

func (s *Server) routes() {
	static, _ := fs.Sub(assets, "assets")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /", s.IndexHandler)
	s.mux.HandleFunc("GET /healthz", s.HealthHandler)
	s.mux.HandleFunc("GET /api/drills", s.ListDrillsHandler)
	s.mux.HandleFunc("POST /api/drills", s.CreateDrillHandler)
	s.mux.HandleFunc("GET /api/drills/{drill_id}", s.GetDrillHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/freeze", s.FreezeHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/activation", s.ActivationHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/start", s.StartHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/observations", s.ObservationHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/findings/{finding_id}/plan", s.RemediationHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/findings/{finding_id}/retest", s.RetestHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/submit-review", s.SubmitReviewHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/review-decision", s.ReviewDecisionHandler)
	s.mux.HandleFunc("GET /api/drills/{drill_id}/timeline", s.TimelineHandler)
	s.mux.HandleFunc("GET /api/drills/{drill_id}/dossier", s.DossierHandler)
	s.mux.HandleFunc("POST /api/drills/{drill_id}/dossier/verify", s.VerifyDossierHandler)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
