package raftnode

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"raft3d/fsm"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

func NewRaftNode(nodeID, raftDir, bindAddr string) (*raft.Raft, *fsm.FSM, error) {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)
	config.SnapshotInterval = 30 * time.Second
	config.SnapshotThreshold = 2

	// Create data directory if needed
	if err := os.MkdirAll(raftDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create raft dir: %v", err)
	}

	// Create stable store
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "stable"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stable store: %v", err)
	}

	// Create log store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "log"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create log store: %v", err)
	}

	// Create snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(raftDir, 3, os.Stderr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create snapshot store: %v", err)
	}

	// Create TCP transport
	transport, err := raft.NewTCPTransport(
		bindAddr,
		nil,
		3,
		10*time.Second,
		os.Stderr,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create tcp transport: %v", err)
	}

	// Create FSM
	fsmInstance := fsm.NewFSM()

	// Instantiate Raft
	raftNode, err := raft.NewRaft(
		config,
		fsmInstance,
		logStore,
		stableStore,
		snapshotStore,
		transport,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create raft: %v", err)
	}

	// Bootstrap cluster if first node
	hasState, _ := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if !hasState {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	}

	return raftNode, fsmInstance, nil
}
