package web

import (
	"net/http"

	"anki-wrapper/internal/domain"
)

func (s *Server) handleTelemetryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vocab := s.getVocabSnapshot()

	var stats *domain.LLMUsageStats
	var recent []domain.LLMUsageRecord

	if s.telemetry != nil {
		stats, _ = s.telemetry.GetUsageStats(ctx)
		recent, _ = s.telemetry.GetRecentUsage(ctx, 50)
	}

	if stats == nil {
		stats = &domain.LLMUsageStats{
			CallsByProvider:   make(map[string]int),
			CallsByOperation:  make(map[string]int),
			TokensByOperation: make(map[string]int64),
		}
	}

	data := map[string]any{
		"ActiveNav":   "telemetry",
		"VocabCount":  len(vocab),
		"Stats":       stats,
		"RecentUsage": recent,
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTelemetryClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	if s.telemetry != nil {
		_ = s.telemetry.ClearUsage(r.Context())
	}

	http.Redirect(w, r, "/telemetry", http.StatusSeeOther)
}
