package utils

import "testing"

func TestSortYAML(t *testing.T) {
	input := []byte(`b: 1
a:
  z: 1
  y:
    - d: 1
      c: 2
`)
	expected := `a:
    y:
        - c: 2
          d: 1
    z: 1
b: 1
`

	out, err := SortYAML(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", out, expected)
	}
}

func TestSortYAMLIsDeterministic(t *testing.T) {
	a, err := SortYAML([]byte("x: 1\ny: 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := SortYAML([]byte("y: 2\nx: 1\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("expected equal output, got:\n%s\nand:\n%s", a, b)
	}
}
