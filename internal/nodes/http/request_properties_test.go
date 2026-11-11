package http

import (
	"testing"

	"github.com/neul-labs/m9m/internal/nodes/base"
)

// findProperty returns the named property from the node description.
func findProperty(t *testing.T, props []base.NodeProperty, name string) base.NodeProperty {
	t.Helper()
	for _, prop := range props {
		if prop.Name == name {
			return prop
		}
	}
	t.Fatalf("property %q not found", name)
	return base.NodeProperty{}
}

// TestHeaderParametersIsTypedFixedCollection pins gap #3: a
// fixedCollection must carry its bucket options and each bucket must
// describe its child fields as typed property defs. Without them the
// editor renders an empty fixedCollection, or infers the row shape
// from stored data instead of from metadata.
func TestHeaderParametersIsTypedFixedCollection(t *testing.T) {
	node := NewHTTPRequestNode()

	prop := findProperty(t, node.Description().Properties, "headerParameters")

	if prop.Type != "fixedCollection" {
		t.Errorf("type = %q, want fixedCollection", prop.Type)
	}
	if len(prop.Options) != 1 {
		t.Fatalf("expected exactly 1 bucket, got %d", len(prop.Options))
	}

	bucket := prop.Options[0]
	if bucket.Name != "parameters" {
		t.Errorf("bucket name = %q, want parameters", bucket.Name)
	}
	if len(bucket.Values) != 2 {
		t.Fatalf("expected 2 child fields, got %d", len(bucket.Values))
	}

	wantFields := map[string]struct {
		typ      string
		required bool
	}{
		"name":  {typ: "string", required: true},
		"value": {typ: "string", required: false},
	}

	for _, child := range bucket.Values {
		want, ok := wantFields[child.Name]
		if !ok {
			t.Errorf("unexpected child field %q", child.Name)
			continue
		}
		if child.Type != want.typ {
			t.Errorf("%s.type = %q, want %q", child.Name, child.Type, want.typ)
		}
		if child.Required != want.required {
			t.Errorf("%s.required = %t, want %t", child.Name, child.Required, want.required)
		}
		if child.DisplayName == "" {
			t.Errorf("%s.displayName must not be empty", child.Name)
		}
	}

	// `multipleValues` is what tells the editor to render the bucket as
	// repeating rows rather than a single record.
	if prop.TypeOptions["multipleValues"] != true {
		t.Errorf("typeOptions.multipleValues = %v, want true", prop.TypeOptions["multipleValues"])
	}
}

// TestFixedCollectionPropAcceptsBuckets guards the builder itself: the
// bucket options a caller passes must survive onto the property.
func TestFixedCollectionPropAcceptsBuckets(t *testing.T) {
	prop := base.FixedCollectionProp("Query Parameters", "queryParameters", "Query rows.", base.Option{
		Name:  "parameters",
		Value: "parameters",
		Values: []base.OptionValue{
			{Name: "name", DisplayName: "Name", Type: "string", Required: true},
		},
	})

	if len(prop.Options) != 1 {
		t.Fatalf("expected the bucket to be kept, got %d options", len(prop.Options))
	}
	if len(prop.Options[0].Values) != 1 {
		t.Fatalf("expected the child field to be kept, got %d", len(prop.Options[0].Values))
	}
	if prop.Options[0].Values[0].Type != "string" {
		t.Errorf("child type = %q, want string", prop.Options[0].Values[0].Type)
	}
}
