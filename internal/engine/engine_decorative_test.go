package engine

import "testing"

// TestIsDecorativeNodeType verifies that the list of node types we
// silently skip during execution covers the common n8n UI exports.
// Decorative nodes must be skipped because their absence would
// otherwise fail an entire workflow that includes them.
func TestIsDecorativeNodeType(t *testing.T) {
	cases := []struct {
		name     string
		nodeType string
		want     bool
	}{
		{"sticky note", "n8n-nodes-base.stickyNote", true},
		{"note", "n8n-nodes-base.note", true},
		{"langchain note", "@n8n/n8n-nodes-langchain.note", true},
		{"webhook is not decorative", "n8n-nodes-base.webhook", false},
		{"set is not decorative", "n8n-nodes-base.set", false},
		{"if is not decorative", "n8n-nodes-base.if", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDecorativeNodeType(tc.nodeType); got != tc.want {
				t.Fatalf("isDecorativeNodeType(%q) = %v, want %v", tc.nodeType, got, tc.want)
			}
		})
	}
}
