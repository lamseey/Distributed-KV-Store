package timer

import (
	"fmt"
	"kvs/models"
	"math/rand"
	"time"
)

const (
	HeartbeatInterval = 100 * time.Millisecond
	ElectionTimeoutMin = 150 // In milliseconds (we use it as int for randomness later)
	ElectionTimeoutMax = 300 // In milliseconds
)

type Timer struct {
	Node *models.Node
	electionTimer *time.Timer
	heartbeatTicker *time.Ticker
	stopElection chan bool
	stopHeartbeat chan bool
}

func NewTimer(n *models.Node) *Timer {
	return &Timer{
		Node: n,
		stopElection: make(chan bool),
		stopHeartbeat: make(chan bool),
	}
}

func (t *Timer) getElectionTimeout() time.Duration {
	return time.Duration(ElectionTimeoutMin + rand.Intn(ElectionTimeoutMax - ElectionTimeoutMin)) * time.Millisecond
}

func (t *Timer) StartElectionTimer() {
	timeout := t.getElectionTimeout()
	t.electionTimer = time.NewTimer(timeout)	// Not the same as the method NewTimer above ^^

	go func () {
		for {
			select {
			case <- t.electionTimer.C:
				// Timeout 
				t.Node.RLock()
				role := t.Node.Role
				t.Node.RUnlock()

				if role != "Leader" {
					fmt.Println("Starting election from", t.Node.ID) // No need to lock, ID never changes ^^
					t.Node.StartElection()

					t.Node.RLock()
					role = t.Node.Role
					t.Node.RUnlock()

					if role == "Leader" {
						// Election won
						t.StopElectionTimer()
						t.StartHeartbeat()
						return
					}
				}
				timeout = t.getElectionTimeout()
				t.electionTimer.Reset(timeout)

			case <- t.stopElection:
				t.electionTimer.Stop()
				return
			}
		}
	}()
}

func (t *Timer) StopElectionTimer() {
	if t.electionTimer != nil {
		go func (){
			t.stopElection <- true
		}()
	}
}

func (t *Timer) StartHeartbeat() {
	t.heartbeatTicker = time.NewTicker(HeartbeatInterval)
	go func(){
		for {
			select{
			case <- t.heartbeatTicker.C:
				t.Node.RLock()
				role := t.Node.Role
				t.Node.RUnlock()
				if role == "Leader" {
					t.sendHeartbeats()
				} else {
					t.StopHeartbeats()
					return
				}
			case <- t.stopHeartbeat:
				t.heartbeatTicker.Stop()
				return
			}

		}
	}()
}

func (t *Timer) sendHeartbeats() {
	t.Node.RLock()
    peers := append([]string(nil), t.Node.Peers...)
    term := t.Node.CurrentTerm
    leaderID := t.Node.ID
    t.Node.RUnlock()

    for _, peer := range peers {
        go func(p string) {
            t.Node.SendAppendEntries(p, term, leaderID, []models.LogEntry{})
        }(peer)
    }
}

func (t *Timer) StopHeartbeats() {
	if t.heartbeatTicker != nil {
		select {
		case t.stopHeartbeat <- true:
		default:
		}
	}
}

func (t *Timer) Run() {
    t.Node.RLock()
    role := t.Node.Role
    t.Node.RUnlock()

    if role == "Leader" {
        t.StartHeartbeat()
    } else {
        t.StartElectionTimer()
    }
}
