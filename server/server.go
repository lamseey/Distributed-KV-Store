package server

import (
	"kvs/models"
	"encoding/json"
	"net/http"
)

type Server struct {
	Node *models.Node
}

func NewServer(node *models.Node) *Server {
	return &Server{Node: node}
}

func (s *Server) HandleRequestVote(w http.ResponseWriter, r http.Request) {
	var req struct {
		Term int			`json:"term"`
		CandidateID int		`json:"candidateID"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return 
	}

	granted := s.Node.RequestVote(req.Term, req.CandidateID)
	response := map[string]bool{"granted":granted}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) HandleAppendEntries(w http.ResponseWriter, r http.Request) {
	var req struct {
		Term int
		LeaderID int
		entries []models.LogEntry
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = s.Node.AppendEntries(req.Term, req.LeaderID, req.entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}