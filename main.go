package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"raft3d/fsm"
	"raft3d/models"
	raftnode "raft3d/raft"

	"github.com/hashicorp/raft"

	"strings"
)

var raftNode *raft.Raft
var fsmInstance *fsm.FSM

func main() {
	if len(os.Args) != 4 {
		log.Fatal("Usage: go run main.go <nodeID> <raftBindAddr> <httpPort>")
	}

	nodeID := os.Args[1]
	raftBindAddr := os.Args[2]
	httpPort := os.Args[3]

	// Initialize Raft
	var err error
	raftNode, fsmInstance, err = raftnode.NewRaftNode(nodeID, "raft-data-"+nodeID, raftBindAddr)
	if err != nil {
		log.Fatalf("Failed to initialize Raft: %v", err)
	}

	// HTTP endpoints
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Raft3D node [%s] - Leader: %t", nodeID, raftNode.State() == raft.Leader)
	})

	http.HandleFunc("/api/v1/printers", printerHandler)
	http.HandleFunc("/api/v1/filaments", filamentHandler)
	http.HandleFunc("/api/v1/print-jobs", printJobHandler)
	http.HandleFunc("/api/v1/print-jobs/", printJobStatusHandler)
	http.HandleFunc("/join", joinHandler)
	http.HandleFunc("/status", statusHandler)

	log.Printf("HTTP server starting on :%s", httpPort)
	log.Fatal(http.ListenAndServe(":"+httpPort, nil))
}

// =================== PRINTER ====================

func printerHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createPrinter(w, r)
	case http.MethodGet:
		listPrinters(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createPrinter(w http.ResponseWriter, r *http.Request) {
	if raftNode.State() != raft.Leader {
		http.Error(w, "This node is not the leader", http.StatusBadRequest)
		return
	}

	var p models.Printer
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if p.ID == "" || p.Company == "" || p.Model == "" {
		http.Error(w, "Missing required fields (id, company, model)", http.StatusBadRequest)
		return
	}

	data, err := json.Marshal(p)
	if err != nil {
		http.Error(w, "Marshal failed", http.StatusInternalServerError)
		return
	}

	cmd := append([]byte("printer:"), data...)
	future := raftNode.Apply(cmd, 5*time.Second)
	if err := future.Error(); err != nil {
		http.Error(w, "Raft apply failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func listPrinters(w http.ResponseWriter, r *http.Request) {
	printers := fsmInstance.GetAllPrinters()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printers)
}

// =================== FILAMENT ====================

func filamentHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createFilament(w, r)
	case http.MethodGet:
		listFilaments(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createFilament(w http.ResponseWriter, r *http.Request) {
	if raftNode.State() != raft.Leader {
		http.Error(w, "This node is not the leader", http.StatusBadRequest)
		return
	}

	var fl models.Filament
	if err := json.NewDecoder(r.Body).Decode(&fl); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if fl.ID == "" || fl.Type == "" || fl.Color == "" || fl.TotalWeightInGrams <= 0 {
		http.Error(w, "Missing required fields or invalid values (id, type, color, total_weight_in_grams)", http.StatusBadRequest)
		return
	}

	fl.RemainingWeightInGrams = fl.TotalWeightInGrams

	data, err := json.Marshal(fl)
	if err != nil {
		http.Error(w, "Marshal failed", http.StatusInternalServerError)
		return
	}

	cmd := append([]byte("filament:"), data...)
	future := raftNode.Apply(cmd, 5*time.Second)
	if err := future.Error(); err != nil {
		http.Error(w, "Raft apply failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(fl)
}

func listFilaments(w http.ResponseWriter, r *http.Request) {
	filaments := fsmInstance.GetAllFilaments()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filaments)
}

// =================== PRINT JOB ====================

func printJobHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createPrintJob(w, r)
	case http.MethodGet:
		listPrintJobs(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createPrintJob(w http.ResponseWriter, r *http.Request) {
	if raftNode.State() != raft.Leader {
		http.Error(w, "This node is not the leader", http.StatusBadRequest)
		return
	}

	var pj models.PrintJob
	if err := json.NewDecoder(r.Body).Decode(&pj); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if pj.ID == "" || pj.PrinterID == "" || pj.FilamentID == "" || pj.PrintWeightInGrams <= 0 {
		http.Error(w, "Missing required fields or invalid values (id, printer_id, filament_id, print_weight_in_grams)", http.StatusBadRequest)
		return
	}

	// Force status to Queued regardless of what client sent
	pj.Status = models.Queued

	// Validate IDs
	printers := fsmInstance.GetAllPrinters()
	foundPrinter := false
	for _, p := range printers {
		if p.ID == pj.PrinterID {
			foundPrinter = true
			break
		}
	}
	if !foundPrinter {
		http.Error(w, "Printer not found", http.StatusBadRequest)
		return
	}

	filaments := fsmInstance.GetAllFilaments()
	foundFilament := false
	for _, f := range filaments {
		if f.ID == pj.FilamentID {
			foundFilament = true
			break
		}
	}
	if !foundFilament {
		http.Error(w, "Filament not found", http.StatusBadRequest)
		return
	}

	// Apply to Raft
	data, err := json.Marshal(pj)
	if err != nil {
		http.Error(w, "Marshal failed", http.StatusInternalServerError)
		return
	}

	cmd := append([]byte("printjob:"), data...)
	future := raftNode.Apply(cmd, 5*time.Second)
	if err := future.Error(); err != nil {
		http.Error(w, "Raft apply failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pj)
}

func listPrintJobs(w http.ResponseWriter, r *http.Request) {
	printJobs := fsmInstance.GetAllPrintJobs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printJobs)
}
func printJobStatusHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 || parts[5] != "status" {
		http.Error(w, "Invalid path. Use /api/v1/print-jobs/{id}/status", http.StatusBadRequest)
		return
	}
	jobID := parts[4]
	updatePrintJobStatus(w, r, jobID)
}

// Updated updatePrintJobStatus
func updatePrintJobStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	if raftNode.State() != raft.Leader {
		http.Error(w, "This node is not the leader", http.StatusBadRequest)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		http.Error(w, "Missing status parameter", http.StatusBadRequest)
		return
	}

	var newStatus models.PrintJobStatus
	switch strings.ToLower(status) {
	case "running":
		newStatus = models.Running
	case "done":
		newStatus = models.Done
	case "canceled":
		newStatus = models.Canceled
	default:
		http.Error(w, "Invalid status. Must be one of: running, done, canceled", http.StatusBadRequest)
		return
	}

	cmd := map[string]interface{}{
		"job_id":     jobID,
		"new_status": newStatus,
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		http.Error(w, "Failed to marshal command", http.StatusInternalServerError)
		return
	}

	raftCmd := append([]byte("statusupdate:"), data...)
	future := raftNode.Apply(raftCmd, 5*time.Second)

	// First check for Raft-level errors
	if err := future.Error(); err != nil {
		http.Error(w, fmt.Sprintf("Raft error: %v", err), http.StatusInternalServerError)
		return
	}

	// Then check for FSM validation errors
	if resp := future.Response(); resp != nil {
		if err, ok := resp.(error); ok {
			http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		} else {
			http.Error(w, "Invalid transition", http.StatusBadRequest)
		}
		return
	}

	// Only reach here if successful
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Job %s successfully updated to %s", jobID, newStatus),
	})
}

// =================== CLUSTER ====================

func joinHandler(w http.ResponseWriter, r *http.Request) {
	if raftNode.State() != raft.Leader {
		http.Error(w, "Only leader can process join requests", http.StatusForbidden)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	future := raftNode.AddVoter(raft.ServerID(req.ID), raft.ServerAddress(req.Address), 0, 0)
	if err := future.Error(); err != nil {
		http.Error(w, "Failed to add voter: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Node %s added at %s\n", req.ID, req.Address)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{
		"node":  os.Args[1],
		"state": raftNode.State().String(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
