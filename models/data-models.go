package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	rpcTimeout     = 2 * time.Second
	rpcMaxAttempts = 2
	rpcRetryDelay  = 50 * time.Millisecond
)

type Node struct {
	ID            int               `json:"id"`
	Address       string            `json:"address"` //e.g., "logcalhost:8080"
	Peers         []string          `json:"peers"`
	Role          string            `json:"role"` //Leader, Follower, Candidate
	Log           []LogEntry        `json:"log"`
	CommitIdx     int               `json:"commit_index"`
	CurrentTerm   int               `json:"current_term"`
	VotedFor      int               `json:"voted_for"` //We keep the ID
	VoteCount     int               `json:"vote_count"`
	Store         map[string]string `json:"store"`      // k-v store
	LastApplied   int               `json:"last_entry"` // Idx of last entry
	DataDir       string            `json:"-"`
	ElectionReset chan struct{}     `json:"-"`
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
	if term < n.CurrentTerm {
		return false
	}

	// The node receiving the request is old, it will become a follower and will vote for the node that did the request
	if term > n.CurrentTerm {
		n.Role = "Follower"
		n.CurrentTerm = term
		n.VotedFor = 0
	}

	// If the node receiving the request already voted, can't vote twice :)
	if n.VotedFor != 0 && n.VotedFor != candidateID {
		return false
	}

	// Bro voted
	n.VotedFor = candidateID
	return true
}

// We do this to send request vote through network

type RequestVoteArgs struct {
	Term        int `json:"term"`
	CandidateID int `json:"candidateID"`
}

type RequestVoteReply struct {
	Granted bool `json:"granted"`
}

func (n *Node) sendRequestVote(peerAddress string, term int, candidateID int) (granted bool) {
	for attempt := 0; attempt < rpcMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		result := make(chan struct {
			granted bool
			err     error
		}, 1)
		go func() {
			client, err := rpc.DialHTTP("tcp", peerAddress)
			if err != nil {
				result <- struct {
					granted bool
					err     error
				}{false, err}
				return
			}
			defer client.Close()

			args := RequestVoteArgs{Term: term, CandidateID: candidateID}
			var reply RequestVoteReply
			err = client.Call("RPCServer.RequestVote", args, &reply)
			result <- struct {
				granted bool
				err     error
			}{reply.Granted, err}
		}()

		select {
		case res := <-result:
			cancel()
			if res.err == nil {
				return res.granted
			}
		case <-ctx.Done():
			cancel()
		}
		time.Sleep(rpcRetryDelay)
	}
	return false
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
		go func(p string) {
			granted := n.sendRequestVote(p, term, candidateID)
			votes <- granted
		}(peer)
	}

	// Let's see if he won the elections
	for range len(peers) {
		if <-votes {
			n.Lock()
			n.VoteCount++
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
		return fmt.Errorf("These entries are old")
	}

	if term > n.CurrentTerm {
		n.CurrentTerm = term
		n.Role = "Follower"
		n.VotedFor = 0
	}

	if n.Role == "Candidate" {
		n.Role = "Follower"
	}

	for _, entry := range entries {
		// append entry
		n.Log = append(n.Log, entry)
		// if it's a set, update the map ^^
		if entry.Command == "SET" {
			if n.Store == nil {
				n.Store = make(map[string]string)
			}
			n.Store[entry.Key] = entry.Value.(string)
		}
	}
	if n.ElectionReset != nil {
		select {
		case n.ElectionReset <- struct{}{}:
		default:
		}
	}
	return nil
}

// Now we want to send through the network the entries

type AppendEntriesArgs struct {
	Term     int        `json:"term"`
	LeaderID int        `json:"leaderID"`
	Entries  []LogEntry `json:"entries"`
}

type AppendEntriesReply struct {
	Success bool `json:"success"`
}

func (n *Node) SendAppendEntries(peerAddress string, term int, leaderID int, entries []LogEntry) bool {
	for attempt := 0; attempt < rpcMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		result := make(chan struct {
			success bool
			err     error
		}, 1)
		go func() {
			client, err := rpc.DialHTTP("tcp", peerAddress)
			if err != nil {
				result <- struct {
					success bool
					err     error
				}{false, err}
				return
			}
			defer client.Close()

			args := AppendEntriesArgs{Term: term, LeaderID: leaderID, Entries: entries}
			var reply AppendEntriesReply
			err = client.Call("RPCServer.AppendEntries", args, &reply)
			result <- struct {
				success bool
				err     error
			}{reply.Success, err}
		}()

		select {
		case res := <-result:
			cancel()
			if res.err == nil {
				return res.success
			}
		case <-ctx.Done():
			cancel()
		}
		time.Sleep(rpcRetryDelay)
	}
	return false

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
		return fmt.Errorf("not a leader")
	}

	entry := LogEntry{
		Term:    n.CurrentTerm,
		Command: "SET",
		Key:     key,
		Value:   val,
	}

	n.Log = append(n.Log, entry)
	if n.Store == nil {
		n.Store = make(map[string]string)
	}
	n.Store[key] = val
	n.Unlock()
	err := n.replicateToFollowers(entry)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) replicateToFollowers(entry LogEntry) error {
	n.RLock()
	peers := append([]string(nil), n.Peers...)
	term := n.CurrentTerm
	leaderID := n.ID
	n.RUnlock()

	majority := (len(peers)+1)/2 + 1
	successCount := 1 // leader itself
	ackChan := make(chan bool, len(peers))

	for _, peer := range peers {
		go func(p string) {
			ok := n.SendAppendEntries(p, term, leaderID, []LogEntry{entry})
			ackChan <- ok
		}(peer)
	}

	for i := 0; i < len(peers); i++ {
		if <-ackChan {
			successCount++
			if successCount >= majority {
				n.Lock()
				n.CommitIdx = len(n.Log) - 1
				n.Unlock()
				return nil
			}
		}
	}

	return fmt.Errorf("failed to reach majority")
}

func (n *Node) SaveState() error {
	n.RLock()
	defer n.RUnlock()

	statePath := n.statePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(statePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(n)
}

func (n *Node) LoadState() error {
	file, err := os.Open(n.statePath())
	if err != nil {
		return nil // first run
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(n)
}

func (n *Node) Compact() error {
	n.Lock()
	if n.CommitIdx < 0 || n.CommitIdx <= n.LastApplied {
		n.Unlock()
		return nil
	}
	storeCopy := make(map[string]string, len(n.Store))
	for k, v := range n.Store {
		storeCopy[k] = v
	}
	snapshot := struct {
		Store       map[string]string `json:"store"`
		CommitIdx   int               `json:"commit_index"`
		LastApplied int               `json:"last_applied"`
		CurrentTerm int               `json:"current_term"`
	}{
		Store:       storeCopy,
		CommitIdx:   n.CommitIdx,
		LastApplied: n.CommitIdx,
		CurrentTerm: n.CurrentTerm,
	}
	if n.CommitIdx+1 < len(n.Log) {
		n.Log = n.Log[n.CommitIdx+1:]
	} else {
		n.Log = nil
	}
	n.LastApplied = n.CommitIdx
	n.Unlock()

	snapshotPath := n.snapshotPath()
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(snapshotPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(snapshot)
}

func (n *Node) statePath() string {
	dir := n.DataDir
	if dir == "" {
		dir = "data"
	}
	return filepath.Join(dir, fmt.Sprintf("node_%d.json", n.ID))
}

func (n *Node) snapshotPath() string {
	dir := n.DataDir
	if dir == "" {
		dir = "data"
	}
	return filepath.Join(dir, fmt.Sprintf("snapshot_%d.json", n.ID))
}
