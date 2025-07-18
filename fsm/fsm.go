package fsm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"raft3d/models"

	"github.com/hashicorp/raft"
)

type FSM struct {
	mu        sync.Mutex
	printers  map[string]models.Printer
	filaments map[string]models.Filament
	printJobs map[string]models.PrintJob
}

func NewFSM() *FSM {
	return &FSM{
		printers:  make(map[string]models.Printer),
		filaments: make(map[string]models.Filament),
		printJobs: make(map[string]models.PrintJob),
	}
}

func (f *FSM) Apply(logEntry *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case bytes.HasPrefix(logEntry.Data, []byte("printer:")):
		var p models.Printer
		if err := json.Unmarshal(logEntry.Data[len("printer:"):], &p); err != nil {
			fmt.Println("[FSM] Failed to unmarshal printer:", err)
			return nil
		}
		f.printers[p.ID] = p
		fmt.Printf("[FSM] Applied printer: %s\n", p.ID)

	case bytes.HasPrefix(logEntry.Data, []byte("filament:")):
		var fl models.Filament
		if err := json.Unmarshal(logEntry.Data[len("filament:"):], &fl); err != nil {
			fmt.Println("[FSM] Failed to unmarshal filament:", err)
			return nil
		}
		f.filaments[fl.ID] = fl
		fmt.Printf("[FSM] Applied filament: %s\n", fl.ID)

	case bytes.HasPrefix(logEntry.Data, []byte("statusupdate:")):
		var cmd struct {
			JobID     string                `json:"job_id"`
			NewStatus models.PrintJobStatus `json:"new_status"`
		}
		if err := json.Unmarshal(logEntry.Data[len("statusupdate:"):], &cmd); err != nil {
			return fmt.Errorf("FSM: unmarshal error: %v", err)
		}

		pj, exists := f.printJobs[cmd.JobID]
		if !exists {
			return fmt.Errorf("FSM: job %s not found", cmd.JobID)
		}

		// Strict transition validation
		switch cmd.NewStatus {
		case models.Running:
			if pj.Status != models.Queued {
				return fmt.Errorf("FSM: invalid transition from %s to Running", pj.Status)
			}
		case models.Done:
			if pj.Status != models.Running {
				return fmt.Errorf("FSM: invalid transition from %s to Done", pj.Status)
			}
			// Deduct filament only when valid
			filament := f.filaments[pj.FilamentID]
			if filament.RemainingWeightInGrams < pj.PrintWeightInGrams {
				return fmt.Errorf("FSM: insufficient filament for job %s", cmd.JobID)
			}
			filament.RemainingWeightInGrams -= pj.PrintWeightInGrams
			f.filaments[pj.FilamentID] = filament
		case models.Canceled:
			if !(pj.Status == models.Queued || pj.Status == models.Running) {
				return fmt.Errorf("FSM: invalid transition from %s to Canceled", pj.Status)
			}
		default:
			return fmt.Errorf("FSM: invalid status %s", cmd.NewStatus)
		}

		pj.Status = cmd.NewStatus
		f.printJobs[cmd.JobID] = pj
		return nil

	case bytes.HasPrefix(logEntry.Data, []byte("printjob:")):
		var pj models.PrintJob
		if err := json.Unmarshal(logEntry.Data[len("printjob:"):], &pj); err != nil {
			fmt.Println("[FSM] Failed to unmarshal print job:", err)
			return nil
		}

		// Validate printer exists
		if _, printerExists := f.printers[pj.PrinterID]; !printerExists {
			fmt.Printf("[FSM] Printer %s not found\n", pj.PrinterID)
			return nil
		}

		// Validate filament exists
		if _, filamentExists := f.filaments[pj.FilamentID]; !filamentExists {
			fmt.Printf("[FSM] Filament %s not found\n", pj.FilamentID)
			return nil
		}

		// Set default status if empty
		if pj.Status == "" {
			pj.Status = models.Queued // Use enum value
		}

		// Store the print job without modifying filament
		f.printJobs[pj.ID] = pj
		fmt.Printf("[FSM] Applied print job: %s\n", pj.ID)
	}

	return nil

}
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return &fsmSnapshot{
		printers:  f.printers,
		filaments: f.filaments,
		printJobs: f.printJobs,
	}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var snapshot struct {
		Printers  map[string]models.Printer
		Filaments map[string]models.Filament
		PrintJobs map[string]models.PrintJob
	}

	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return fmt.Errorf("failed to decode snapshot: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.printers = snapshot.Printers
	f.filaments = snapshot.Filaments
	f.printJobs = snapshot.PrintJobs

	return nil
}

type fsmSnapshot struct {
	printers  map[string]models.Printer
	filaments map[string]models.Filament
	printJobs map[string]models.PrintJob
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(struct {
		Printers  map[string]models.Printer
		Filaments map[string]models.Filament
		PrintJobs map[string]models.PrintJob
	}{
		Printers:  s.printers,
		Filaments: s.filaments,
		PrintJobs: s.printJobs,
	})
	if err != nil {
		sink.Cancel()
		return fmt.Errorf("failed to persist snapshot: %v", err)
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}

func (f *FSM) GetAllPrinters() []models.Printer {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []models.Printer
	for _, p := range f.printers {
		result = append(result, p)
	}
	return result
}

func (f *FSM) GetAllPrintJobs() []models.PrintJob {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []models.PrintJob
	for _, pj := range f.printJobs {
		result = append(result, pj)
	}
	return result
}

func (f *FSM) GetAllFilaments() []models.Filament {
	f.mu.Lock()
	defer f.mu.Unlock()

	var result []models.Filament
	for _, fl := range f.filaments {
		result = append(result, fl)
	}
	return result
}

func (f *FSM) GetFilamentByID(id string) (*models.Filament, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fl, exists := f.filaments[id]
	if !exists {
		return nil, fmt.Errorf("filament not found")
	}
	return &fl, nil
}
