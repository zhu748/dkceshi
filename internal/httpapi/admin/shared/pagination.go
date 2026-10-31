package shared

// PageBounds validates the page before multiplying, including on 32-bit builds.
// Requests beyond the final page return an empty slice at the end of the list.
func PageBounds(page, pageSize, total int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if page > (total-1)/pageSize+1 {
		return total, total
	}
	start = (page - 1) * pageSize
	return start, start + min(pageSize, total-start)
}
