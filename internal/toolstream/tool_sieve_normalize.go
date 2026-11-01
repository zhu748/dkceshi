package toolstream

import "ds2api/internal/toolcall"

func (s *State) ensureNormalizer() *toolcall.MarkerNormalizer {
	if s.Marker == "" {
		return nil
	}
	if s.normalizer == nil {
		s.normalizer = toolcall.NewMarkerNormalizer(s.Marker)
	}
	return s.normalizer
}

// normalizeIncoming 把新流入的文本从调用者标识翻译回 EPSE，并在必要时保留
// 可能跨越分片的标识前缀。
func (s *State) normalizeIncoming(chunk string) string {
	n := s.ensureNormalizer()
	if n == nil {
		return chunk
	}
	return n.Write(chunk)
}

// flushNormalizer 在流结束时释放被保留的标识残片。
func (s *State) flushNormalizer() string {
	if s.normalizer == nil {
		return ""
	}
	return s.normalizer.Flush()
}
