package tooldiagnostics

// Publish immutable settings only after a successful save. Completion setup
// must not wait behind another request's detail serialization or disk sync.
func (s *Store) publishSettingsLocked() {
	value := s.state.Settings
	if s.loadError != nil {
		value.Enabled = false
		repair := DefaultRepairSettings()
		value.Repair = &repair
		value.RepairError = "repair settings could not be loaded"
	}
	if value.Repair != nil {
		repair := *value.Repair
		repair.UnavailableReason = value.RepairError
		repair.RetryCount = copyRetryCount(repair.RetryCount)
		repair.ExtraBody = append([]byte(nil), repair.ExtraBody...)
		value.Repair = &repair
	}
	s.settingsCache.Store(&value)
}
