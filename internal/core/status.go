package core

// StatusUpdater receives progress updates during generation.
// Implementations must be safe for concurrent use.
type StatusUpdater interface {
	UpdateStatus(msg string)
}

// NoopStatus is a StatusUpdater that discards all updates.
type NoopStatus struct{}

// UpdateStatus implements StatusUpdater.
func (NoopStatus) UpdateStatus(string) {}

// UpdateGenerateStatus is a convenience for providers to push status updates
// through GenerateOptions without nil-checking.
func UpdateGenerateStatus(opts *GenerateOptions, msg string) {
	if opts != nil && opts.Status != nil {
		opts.Status.UpdateStatus(msg)
	}
}
