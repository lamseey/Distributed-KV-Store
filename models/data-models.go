package models

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"
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

// We do this to send request vote through network

func (n *Node) sendRequestVote(peerAddress string, term int, candidateID int) (granted bool){
	mapToSend := map[string]int {
		"term":term, 
		"candidateID": candidateID,
	}

	body, _ := json.Marshal(mapToSend)

	// We add timeout of 2 seconds

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post("http://"+peerAddress+"/vote", "application/json", bytes.NewBuffer(body))

	if err != nil {return false}
	defer resp.Body.Close()

	var result map[string]bool
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return false
	}
	return result["granted"]

}

func (n *Node) StartElection() {
	// We change these fields for our candidate
	n.Lock()
	n.CurrentTerm++
	n.Role = "Candidate"
	n.VotedFor = n.ID
	n.VoteCount = 1
	term := n.CurrentTerm
	candidateID := n.ID
	peers := append([]string(nil), n.Peers...) // Copy peers
	n.Unlock()

	majority := (len(peers) / 2) + 1
	votes := make(chan bool, len(peers))

	for _, peer := range peers {
		go func(p string){
			granted := n.sendRequestVote(p, term, candidateID)
			votes <- granted
		}(peer)
	}

	// Let's see if he won the elections
	for range len(peers){
		if <- votes {
			n.Lock()
			n.VoteCount ++
			// We check if n.Role == "Candidate" because we can become Follower if someone appended entries
			if n.VoteCount >= majority && n.Role == "Candidate" {
				n.Role = "Leader"
				n.Unlock()
				return
			}
			n.Unlock()
		}
	}
}

func (n *Node) AppendEntries(term int, leaderID int, entries []LogEntry) error {
	n.Lock()
	defer n.Unlock()

	if term < n.CurrentTerm {
		// Old entries
		return nil
	}

	if term > n.CurrentTerm {
		n.CurrentTerm = term
		n.Role = "Follower"
		n.VotedFor = 0
	}

	if n.Role == "Candidate" {
		n.Role = "Follower"
	}

	for _, entry := range(entries) {
		// append entry
		n.Log = append(n.Log, entry)
		// if it's a set, update the map ^^
		if entry.Command == "SET" {
			n.Store[entry.Key] = entry.Value.(string)
		}
	}
	return nil
}

// Now we want to send through the network the entries 

func (n *Node) SendAppendEntries(peerAddress string, term int, leaderID int, entries []LogEntry) bool {
	mapToSend := map[string]any {
		"term"	  :term,
		"leaderID":leaderID,
		"entries" :entries,
	}

	reqBody, _ := json.Marshal(mapToSend)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post("http://" + peerAddress + "/append", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return false
	}
	
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (n *Node) Get(key string) (string, bool) {
	n.RLock()
	defer n.RUnlock()

	val, ok := n.Store[key]
	return val, ok 
}

func (n *Node) Set(key string, val string) error {
	n.Lock()

	if n.Role != "Leader" {
		n.Unlock()
		return nil
	}

	_, ok := n.Store[key]
	if !ok {
		n.Unlock()
		return nil
	}

	entry := LogEntry {
		Term   : n.CurrentTerm,
		Command: "SET",
		Key    : key,
		Value  : val,
	}

	n.Log = append(n.Log, entry)
	n.Store[key] = val
	n.Unlock()

	return nil
}


