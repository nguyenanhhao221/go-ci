package add

import "testing"

func TestAdd(t *testing.T) {
	res := add(3, 5)
	exp := 8

	if res != exp {
		t.Errorf("Expecte: %d, got %d", exp, res)
	}
}
