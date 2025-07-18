package models

type FilamentType string

const (
	PLA  FilamentType = "PLA"
	PETG FilamentType = "PETG"
	ABS  FilamentType = "ABS"
	TPU  FilamentType = "TPU"
)

type Filament struct {
	ID                     string       `json:"id"`
	Type                   FilamentType `json:"type"`
	Color                  string       `json:"color"`
	TotalWeightInGrams     int          `json:"total_weight_in_grams"`
	RemainingWeightInGrams int          `json:"remaining_weight_in_grams"`
}
