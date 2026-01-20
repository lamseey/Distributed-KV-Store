package models

import (
	"sync"
)

type Node struct {
	ID          int        `json:"id"`
	Address     string     `json:"address"` //e.g., "logcalhost:8080"
	Peers       []string   `json:"peers"`
	Role        string     `json:"role"` //Leader, Follower, Candidate
	Log         []LogEntry `json:"log"`
	CommitIdx   int        `json:"commit_index"`
	CurrentTerm int        `json:"current_term"`
	VotedFor    int        `json:"voted_for"` //We keep the ID
	mu          sync.RWMutex
}

type LogEntry struct {
	Term    int         `json:"term"`
	Command string      `json:"command"` // e.g., "SET key value"
	Key     string      `json:"key"`
	Value   interface{} `json:"value"`
}

type Consensus interface {
	RequestVote(term int, candidateID int) (granted bool)
	AppendEntries(term int, leaderID int, entries []LogEntry) error
	StartElection()
}

func (n *Node) RequestVote(term int, candidateID int) (granted bool) {

}

func (n *Node) StartElection() {
	n.CurrentTerm++
	n.Role = "Candidate"
	n.VotedFor = n.ID
	go func() {
		if n.RequestVote(n.CurrentTerm, n.ID) {

		}
	}()
}
