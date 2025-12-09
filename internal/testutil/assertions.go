package testutil

import (
	"encoding/json"
	"reflect"
	"testing"
)

// AssertJSONEqual asserts that two JSON strings are semantically equal.
func AssertJSONEqual(t *testing.T, expected, actual string) {
	t.Helper()

	var expectedJSON, actualJSON interface{}

	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		t.Fatalf("failed to parse expected JSON: %v", err)
	}

	if err := json.Unmarshal([]byte(actual), &actualJSON); err != nil {
		t.Fatalf("failed to parse actual JSON: %v", err)
	}

	if !reflect.DeepEqual(expectedJSON, actualJSON) {
		expectedPretty, _ := json.MarshalIndent(expectedJSON, "", "  ")
		actualPretty, _ := json.MarshalIndent(actualJSON, "", "  ")
		t.Errorf("JSON not equal:\nExpected:\n%s\n\nActual:\n%s", expectedPretty, actualPretty)
	}
}

// AssertContains asserts that a string contains a substring.
func AssertContains(t *testing.T, str, substr string) {
	t.Helper()
	if len(str) == 0 || len(substr) == 0 {
		if len(substr) > 0 {
			t.Errorf("string does not contain %q", substr)
		}
		return
	}

	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return
		}
	}
	t.Errorf("string %q does not contain %q", str, substr)
}

// AssertNotContains asserts that a string does not contain a substring.
func AssertNotContains(t *testing.T, str, substr string) {
	t.Helper()
	if len(str) == 0 || len(substr) == 0 {
		return
	}

	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			t.Errorf("string %q contains %q but should not", str, substr)
			return
		}
	}
}

// AssertNoError asserts that an error is nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// AssertError asserts that an error is not nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected error but got nil")
	}
}

// AssertEqual asserts that two values are equal.
func AssertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("not equal:\nExpected: %v (%T)\nActual:   %v (%T)", expected, expected, actual, actual)
	}
}

// AssertNotEqual asserts that two values are not equal.
func AssertNotEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if reflect.DeepEqual(expected, actual) {
		t.Errorf("values should not be equal: %v", expected)
	}
}

// AssertNil asserts that a value is nil.
func AssertNil(t *testing.T, value interface{}) {
	t.Helper()
	if value != nil && !reflect.ValueOf(value).IsNil() {
		t.Errorf("expected nil but got: %v", value)
	}
}

// AssertNotNil asserts that a value is not nil.
func AssertNotNil(t *testing.T, value interface{}) {
	t.Helper()
	if value == nil || reflect.ValueOf(value).IsNil() {
		t.Error("expected not nil but got nil")
	}
}

// AssertTrue asserts that a value is true.
func AssertTrue(t *testing.T, value bool) {
	t.Helper()
	if !value {
		t.Error("expected true but got false")
	}
}

// AssertFalse asserts that a value is false.
func AssertFalse(t *testing.T, value bool) {
	t.Helper()
	if value {
		t.Error("expected false but got true")
	}
}

// AssertLen asserts that a collection has the expected length.
func AssertLen(t *testing.T, collection interface{}, length int) {
	t.Helper()
	v := reflect.ValueOf(collection)
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String, reflect.Chan:
		if v.Len() != length {
			t.Errorf("expected length %d but got %d", length, v.Len())
		}
	default:
		t.Errorf("AssertLen called on non-collection type: %T", collection)
	}
}

// AssertEmpty asserts that a collection is empty.
func AssertEmpty(t *testing.T, collection interface{}) {
	t.Helper()
	AssertLen(t, collection, 0)
}

// AssertNotEmpty asserts that a collection is not empty.
func AssertNotEmpty(t *testing.T, collection interface{}) {
	t.Helper()
	v := reflect.ValueOf(collection)
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String, reflect.Chan:
		if v.Len() == 0 {
			t.Error("expected non-empty collection but got empty")
		}
	default:
		t.Errorf("AssertNotEmpty called on non-collection type: %T", collection)
	}
}
