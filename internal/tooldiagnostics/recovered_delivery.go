package tooldiagnostics

// The terminal receipt may have committed just before process cleanup stopped.
// Preserve that durable success instead of inventing a transport interruption.
func recoverWrittenResponse(record *Record) bool {
	if len(record.Attempts) == 0 {
		return false
	}
	a := &record.Attempts[len(record.Attempts)-1]
	if a.ResponseWrittenAt == 0 || a.Result != "success" {
		return false
	}
	record.Status, record.Delivery = a.Method+"_success", "written"
	if len(record.Attempts) > 1 {
		record.Status = "regenerated_success"
	}
	record.CompletedAt, record.UpdatedAt = a.ResponseWrittenAt, a.ResponseWrittenAt
	return true
}
