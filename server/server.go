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
        Term     int                `json:"term"`
        LeaderID int                `json:"leaderID"`
        Entries  []models.LogEntry  `json:"entries"`
    }

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = s.Node.AppendEntries(req.Term, req.LeaderID, req.Entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleGet(w http.ResponseWriter, r http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key parameter needed", http.StatusBadRequest)
		return
	}

	val, ok := s.Node.Get(key)

	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	response := map[string]string {"key":key, "value":val}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) HandleSet(w http.ResponseWriter, r http.Request) {
	var req struct {
		Key string		`json:"key"`
		Value string	`json:"value"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.Node.RLock()
	isLeader := s.Node.Role == "Leader"
	s.Node.RUnlock()

	if !isLeader {
		http.Error(w, "not a leader", http.StatusBadRequest)
		return
	}

	err = s.Node.Set(req.Key, req.Value)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}