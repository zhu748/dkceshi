package main

// Dependencies expose both void and error-returning cleanup APIs.
func closeBridge(value any) {
	switch v := value.(type) {
	case interface{ Close() error }:
		if err := v.Close(); err != nil {
			diagnostic("cleanup_close_failed")
		}
	case interface{ Close() }:
		v.Close()
	}
}
func closeWriteBridge(value any) {
	if v, ok := value.(interface{ CloseWrite() error }); ok {
		if err := v.CloseWrite(); err != nil {
			diagnostic("cleanup_close_write_failed")
		}
	}
}
