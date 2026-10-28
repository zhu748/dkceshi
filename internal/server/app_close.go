package server

// Close stops background work before releasing transports and flushing ledgers.
func (a *App) Close() {
	if a == nil {
		return
	}
	if a.KeepAlive != nil {
		a.KeepAlive.Close()
	}
	if a.DS != nil {
		a.DS.Close()
	}
	if a.Mihomo != nil {
		a.Mihomo.Stop()
	}
	if a.Pool != nil {
		a.Pool.Close()
	}
	if a.Usage != nil {
		a.Usage.Close()
	}
}
