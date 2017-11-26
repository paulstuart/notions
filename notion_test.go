package notions

import (
	"testing"
)

/*
func TestLineAppend(t *testing.T) {
	item := &Item{text: "hello"}
	if err := item.TextAppend(item.version, " world"); err != nil {
		t.Fatal(err)
	}
}
*/

type testRE struct {
	data string
	ok   bool
}

func TestRegexp(t *testing.T) {
	//tests := []struct{data []byte, ok bool}{
	tests := []testRE{
		{"1. hey", true},
		{"b. you", true},
		{"not you", false},
	}

	for _, test := range tests {
		m := numbered.Match([]byte(test.data))
		if m != test.ok {
			t.Errorf("With %s, expected %t but got %t", string(test.data), test.ok, m)
		}
	}
}
