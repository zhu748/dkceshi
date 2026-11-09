package files

import (
	"fmt"
	"net/http"
)

// Original references and every successfully uploaded file must remain reachable
// from the transformed input. Never commit a successful but incomplete batch.
func (s *inlineUploadState) checkReferenceCoverage(ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	missing := false
	for _, id := range s.expectedRefs {
		missing = missing || !seen[id]
	}
	for _, file := range s.pendingFiles {
		missing = missing || file == nil || !seen[file.ID]
	}
	if missing {
		err := fmt.Errorf("file context references were lost during preprocessing")
		return &inlineFileUploadError{status: http.StatusInternalServerError, message: err.Error(), err: err}
	}
	return nil
}
