package web

import (
	"net/http"

	"shelter-drill-gate/internal/workflow"
)

func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ListDrillsHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.workflow.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drills": result})
}

func (s *Server) CreateDrillHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) GetDrillHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.workflow.Get(r.Context(), r.PathValue("drill_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) FreezeHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.Mutation
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.Freeze(r.Context(), r.PathValue("drill_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) ActivationHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.ActivationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.RegisterActivation(r.Context(), r.PathValue("drill_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) StartHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.Mutation
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.Start(r.Context(), r.PathValue("drill_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) ObservationHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.ObservationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.SubmitObservation(r.Context(), r.PathValue("drill_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) RemediationHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.RemediationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.PlanRemediation(r.Context(), r.PathValue("drill_id"), r.PathValue("finding_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) RetestHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.RetestInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.SubmitRetest(r.Context(), r.PathValue("drill_id"), r.PathValue("finding_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) SubmitReviewHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.Mutation
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.SubmitReview(r.Context(), r.PathValue("drill_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) ReviewDecisionHandler(w http.ResponseWriter, r *http.Request) {
	var input workflow.ReviewDecisionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.workflow.ReviewDecision(r.Context(), r.PathValue("drill_id"), input)
	respondMutation(w, result, err)
}

func (s *Server) TimelineHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.workflow.Timeline(r.Context(), r.PathValue("drill_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": result})
}

func (s *Server) DossierHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.workflow.Dossier(r.Context(), r.PathValue("drill_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) VerifyDossierHandler(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, err)
		return
	}
	result := s.workflow.VerifyDossier(r.Context(), r.PathValue("drill_id"))
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusConflict
	}
	writeJSON(w, status, result)
}

func respondMutation(w http.ResponseWriter, result workflow.DrillView, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
