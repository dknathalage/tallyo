package app

import "testing"

func TestEnvBool(t *testing.T) {
	const key = "TALLYO_TEST_ENVBOOL"

	// Unset key falls through to the default (both directions).
	if got := EnvBool(key+"_NEVER_SET", true); got != true {
		t.Errorf("unset: got %v, want true", got)
	}
	if got := EnvBool(key+"_NEVER_SET", false); got != false {
		t.Errorf("unset: got %v, want false", got)
	}

	cases := []struct {
		val  string
		def  bool
		want bool
	}{
		{val: "", def: true, want: true}, // empty → default
		{val: "true", def: false, want: true},
		{val: "1", def: false, want: true},
		{val: "false", def: true, want: false},
		{val: "0", def: true, want: false},
		{val: "garbage", def: true, want: true}, // unparseable → default
	}
	for _, c := range cases {
		t.Setenv(key, c.val)
		if got := EnvBool(key, c.def); got != c.want {
			t.Errorf("EnvBool(%q, def=%v) = %v, want %v", c.val, c.def, got, c.want)
		}
	}
}
