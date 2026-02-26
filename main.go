package main

import (
	"flag"
	"fmt"
	"kvs/models"
	"kvs/server"
	"kvs/timer"
	"strings"
	"time"
)

func main() {
	id := flag.Int("id", 1, "node id")
	addr := flag.String("addr", "localhost:8001", "node address")
	peersFlag := flag.String("peers", "", "comma-separated peer addresses")
	dataDir := flag.String("data", "data", "data directory")
	compactInterval := flag.Duration("compact", 0, "compaction interval")
	flag.Parse()

	var peers []string
	if *peersFlag != "" {
		for _, peer := range strings.Split(*peersFlag, ",") {
			peer = strings.TrimSpace(peer)
			if peer != "" {
				peers = append(peers, peer)
			}
		}
	}

	node := &models.Node{
		ID:            *id,
		Address:       *addr,
		Peers:         peers,
		Role:          "Follower",
		Store:         make(map[string]string),
		CommitIdx:     -1,
		LastApplied:   -1,
		DataDir:       *dataDir,
		ElectionReset: make(chan struct{}, 1),
	}

	if err := node.LoadState(); err != nil {
		fmt.Println("load state error:", err)
	}
	if node.Store == nil {
		node.Store = make(map[string]string)
	}

	s := server.NewServer(node)
	s.StartREST()
	go func() {
		if err := s.Start(); err != nil {
			fmt.Println("server error:", err)
		}
	}()

	clusterTimer := timer.NewTimer(node)
	clusterTimer.Run()

	go func() {
		persistTicker := time.NewTicker(5 * time.Second)
		defer persistTicker.Stop()
		for range persistTicker.C {
			if err := node.SaveState(); err != nil {
				fmt.Println("save state error:", err)
			}
		}
	}()

	if *compactInterval > 0 {
		go func() {
			compactTicker := time.NewTicker(*compactInterval)
			defer compactTicker.Stop()
			for range compactTicker.C {
				if err := node.Compact(); err != nil {
					fmt.Println("compact error:", err)
				}
			}
		}()
	}

	select {}
}
