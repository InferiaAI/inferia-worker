package dockerclient

import "testing"

func TestTrimAfter(t *testing.T) {
	cases := map[string]string{
		"8000/tcp": "8000",
		"8000":     "8000",
		"":         "",
		"/tcp":     "",
		"abc/":     "abc",
	}
	for in, want := range cases {
		if got := trimAfter(in, '/'); got != want {
			t.Errorf("trimAfter(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNewEngine_ReturnsClient(t *testing.T) {
	// Construct with an unreachable host: the constructor only opens an HTTP
	// transport; it does not perform a Ping. We still expect a non-nil Client.
	c, err := NewEngine("tcp://127.0.0.1:1")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if c == nil {
		t.Errorf("expected non-nil client")
	}
}
