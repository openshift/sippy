package dataloader

type DataLoader interface {
	// Name returns a friendly name identifier
	Name() string

	// Load initiates the data loading process.
	Load()

	// Errors returns a slice of errors that occurred during the data loading process.
	Errors() []error
}
