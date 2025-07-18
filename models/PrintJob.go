package models

type PrintJobStatus string

const (
	Queued   PrintJobStatus = "Queued"
	Running  PrintJobStatus = "Running"
	Done     PrintJobStatus = "Done"
	Canceled PrintJobStatus = "Canceled"
)

type PrintJob struct {
	ID                 string         `json:"id"`
	PrinterID          string         `json:"printer_id"`
	FilamentID         string         `json:"filament_id"`
	File               string         `json:"file"`
	PrintWeightInGrams int            `json:"print_weight_in_grams"`
	Status             PrintJobStatus `json:"status,omitempty"`
}
