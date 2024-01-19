package sockstat

import "testing"

func TestUint32(t *testing.T) {
	examples := []struct {
		input  string
		output uint32
	}{
		{"", 0},
		{"123", 0},
		{"abc", 0},
		{"bool:true", 0},
		{"bool:false", 0},
		{"uint:-1", 0},
		{"uint:1", 1},
		{"uint:4294967295", 4294967295},
		{"uint:4294967296", 0},
		{"uint:4294967297", 0},
		{"uint:abc", 0},
		{"uint: 1", 0},
		{"uint:1 ", 0},
	}

	for _, ex := range examples {
		actual := Uint32Value(ex.input)
		if actual != ex.output {
			t.Errorf("Uint32Value(%q): expected %d, but was %d", ex.input, ex.output, actual)
		}
	}
}

func TestString(t *testing.T) {
	examples := []struct {
		input  string
		output string
	}{
		{"", ""},
		{"123", "123"},
		{"abc", "abc"},
		{"bool:true", "true"},
		{"bool:false", "false"},
		{"uint:-1", "-1"},
		{"uint:1", "1"},
		{"uint:4294967295", "4294967295"},
		{"uint:4294967296", "4294967296"},
		{"bool:uint:anything", "uint:anything"},
		{"uint:bool:anything", "bool:anything"},
		{"anything:uint:bool", "anything:uint:bool"},
	}

	for _, ex := range examples {
		actual := StringValue(ex.input)
		if actual != ex.output {
			t.Errorf("StringValue(%q): expected %q, but was %q", ex.input, ex.output, actual)
		}
	}
}
