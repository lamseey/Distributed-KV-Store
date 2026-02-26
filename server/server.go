package server

import (
	"encoding/json"
	"fmt"
	"kvs/models"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"time"
)

type Server struct {
	Node *models.Node
}

func NewServer(node *models.Node) *Server {
	return &Server{Node: node}
}

// RPC Server wrapper for registering methods
type RPCServer struct {
	Server *Server
}

// RPC method for RequestVote
func (rs *RPCServer) RequestVote(args *models.RequestVoteArgs, reply *models.RequestVoteReply) error {
	granted := rs.Server.Node.RequestVote(args.Term, args.CandidateID)
	reply.Granted = granted
	return nil
}

// RPC method for AppendEntries
func (rs *RPCServer) AppendEntries(args *models.AppendEntriesArgs, reply *models.AppendEntriesReply) error {
	err := rs.Server.Node.AppendEntries(args.Term, args.LeaderID, args.Entries)
	if err != nil {
		reply.Success = false
		return err
	}
	reply.Success = true
	return nil
}

// Get RPC Args and Reply
type GetArgs struct {
	Key string
}

type GetReply struct {
	Value string
	Found bool
}

// RPC method for Get
func (rs *RPCServer) Get(args *GetArgs, reply *GetReply) error {
	val, ok := rs.Server.Node.Get(args.Key)
	reply.Value = val
	reply.Found = ok
	return nil
}

// Set RPC Args and Reply
type SetArgs struct {
	Key   string
	Value string
}

type SetReply struct {
	Success bool
	Error   string
}

// RPC method for Set
func (rs *RPCServer) Set(args *SetArgs, reply *SetReply) error {
	rs.Server.Node.RLock()
	isLeader := rs.Server.Node.Role == "Leader"
	rs.Server.Node.RUnlock()

	if !isLeader {
		reply.Success = false
		reply.Error = "not a leader"
		return fmt.Errorf("not a leader")
	}

	err := rs.Server.Node.Set(args.Key, args.Value)
	if err != nil {
		reply.Success = false
		reply.Error = err.Error()
		return err
	}

	reply.Success = true
	return nil
}

// Status RPC Args and Reply
type StatusArgs struct{}

type StatusReply struct {
	ID          int
	Address     string
	Role        string
	Term        int
	VotedFor    int
	LogSize     int
	CommitIndex int
}

// RPC method for Status
func (rs *RPCServer) Status(args *StatusArgs, reply *StatusReply) error {
	rs.Server.Node.RLock()
	reply.ID = rs.Server.Node.ID
	reply.Address = rs.Server.Node.Address
	reply.Role = rs.Server.Node.Role
	reply.Term = rs.Server.Node.CurrentTerm
	reply.VotedFor = rs.Server.Node.VotedFor
	reply.LogSize = len(rs.Server.Node.Log)
	reply.CommitIndex = rs.Server.Node.CommitIdx
	rs.Server.Node.RUnlock()
	return nil
}

func (s *Server) Start() error {
	rpcServer := &RPCServer{Server: s}

	// Register RPC server
	err := rpc.Register(rpcServer)
	if err != nil {
		return fmt.Errorf("failed to register RPC server: %v", err)
	}

	// Handle RPC over HTTP
	rpc.HandleHTTP()

	// Create listener
	listener, err := net.Listen("tcp", s.Node.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", s.Node.Address, err)
	}

	fmt.Println("Node", s.Node.ID, "starting RPC server on", s.Node.Address)
	return http.Serve(listener, nil)
}

func (s *Server) StartREST() {
	http.HandleFunc("/get/", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[len("/get/"):]
		val, ok := s.Node.Get(key)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Write([]byte(val))
	})

	http.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		var key, val string
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var payload struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				key = payload.Key
				val = payload.Value
			}
		}
		if key == "" {
			key = r.URL.Query().Get("key")
			val = r.URL.Query().Get("value")
		}
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}

		err := s.Node.Set(key, val)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte("ok"))
	})

	http.HandleFunc("/cluster/status", func(w http.ResponseWriter, r *http.Request) {
		peers := append([]string(nil), s.Node.Peers...)

		s.Node.RLock()
		local := StatusReply{
			ID:          s.Node.ID,
			Address:     s.Node.Address,
			Role:        s.Node.Role,
			Term:        s.Node.CurrentTerm,
			VotedFor:    s.Node.VotedFor,
			LogSize:     len(s.Node.Log),
			CommitIndex: s.Node.CommitIdx,
		}
		s.Node.RUnlock()

		var builder strings.Builder
		fmt.Fprintf(&builder, "node=%d addr=%s role=%s term=%d commit=%d\n",
			local.ID, local.Address, local.Role, local.Term, local.CommitIndex)
		for _, peer := range peers {
			status, err := fetchStatus(peer)
			if err != nil {
				fmt.Fprintf(&builder, "addr=%s error=%s\n", peer, err.Error())
				continue
			}
			fmt.Fprintf(&builder, "node=%d addr=%s role=%s term=%d commit=%d\n",
				status.ID, status.Address, status.Role, status.Term, status.CommitIndex)
		}
		w.Write([]byte(builder.String()))
	})
}

func fetchStatus(address string) (StatusReply, error) {
	result := make(chan struct {
		reply StatusReply
		err   error
	}, 1)
	go func() {
		client, err := rpc.DialHTTP("tcp", address)
		if err != nil {
			result <- struct {
				reply StatusReply
				err   error
			}{StatusReply{}, err}
			return
		}
		defer client.Close()
		var reply StatusReply
		err = client.Call("RPCServer.Status", &StatusArgs{}, &reply)
		result <- struct {
			reply StatusReply
			err   error
		}{reply, err}
	}()

	select {
	case res := <-result:
		return res.reply, res.err
	case <-time.After(2 * time.Second):
		return StatusReply{}, fmt.Errorf("timeout")
	}
}
