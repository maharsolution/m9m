package storage

// workflowHasAllTags reports whether the workflow carries EVERY tag ID
// listed in required (AND semantics).
//
// The comparison is exact — tag IDs are opaque identifiers, so the Go
// backends must agree with the SQL backends, which match the IDs
// inside the JSON `tags` column with a case-sensitive comparison.
//
// An empty `required` list matches every workflow; the callers skip
// the check entirely in that case.
func workflowHasAllTags(workflowTags []string, required []string) bool {
	if len(required) == 0 {
		return true
	}

	for _, wanted := range required {
		found := false
		for _, carried := range workflowTags {
			if carried == wanted {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
