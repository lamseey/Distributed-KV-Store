package models

import (
	"sync"
)

type Node struct {		
	ID          int         		`json:"id"`
	Address     string      		`json:"address"` //e.g., "logcalhost:8080"
	Peers       []string    		`json:"peers"`
	Role        string      		`json:"role"` //Leader, Follower, Candidate
	Log         []LogEntry  		`json:"log"`
	CommitIdx   int         		`json:"commit_index"`
	CurrentTerm int         		`json:"current_term"`
	VotedFor    int         		`json:"voted_for"` //We keep the ID
	VoteCount 	int  	 			`json:"vote_count"`
	Store 		map[string]string	`json:"store"` // k-v store
	LastApplied int					`json:"last_entry"` // Idx of last entry
	sync.RWMutex
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
	n.Lock()
	defer n.Unlock()
	// The node that did the request is old 
	if (term < n.CurrentTerm) {
		return false
	}

	// The node receiving the request is old, it will become a follower and will vote for the node that did the request
	if (term > n.CurrentTerm){
		n.Role = "Follower"
		n.CurrentTerm = term
		n.VotedFor = 0
	}

	// If the node receiving the request already voted, can't vote twice :)
	if (n.VotedFor != 0 && n.VotedFor != candidateID) {
		return false
	}

	// Bro voted
	n.VotedFor = candidateID
	return true
}

func (n *Node) StartElection() {
	n.CurrentTerm++
	n.Role = "Candidate"
	n.VotedFor = n.ID
	go func() {
		
	}()
}
