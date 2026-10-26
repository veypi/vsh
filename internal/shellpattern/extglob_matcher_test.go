package pattern

import "testing"

func TestMatchInvalidUTF8AndBadPatternsDoNotPanic(t *testing.T) {
	raw := string([]byte{0x80})
	for _, tc := range []struct {
		name      string
		pat       string
		candidate string
		mode      Mode
		want      bool
		wantErr   bool
	}{
		{name: "raw literal", pat: raw, candidate: raw, mode: EntireString, want: true},
		{name: "raw candidate", pat: "*", candidate: raw, mode: EntireString, want: true},
		{name: "raw byte in filename", pat: "a?c", candidate: "a" + raw + "c", mode: Filenames | EntireString, want: true},
		{name: "filename star still rejects slash", pat: "a*c", candidate: "a/" + raw + "c", mode: Filenames | EntireString, want: false},
		{name: "bad class", pat: "[", candidate: "x", mode: EntireString, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Match panicked: %v", r)
				}
			}()
			got, err := Match(tc.pat, tc.candidate, tc.mode)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Match error=%v, wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Match=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtendedPatternMatcherCompileFailureDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExtendedPatternMatcher panicked: %v", r)
		}
	}()
	if _, err := ExtendedPatternMatcher(string([]byte{0x80}), EntireString); err == nil {
		t.Fatal("expected invalid UTF-8 regexp compile error")
	}
}

func TestExtendedPatternMatcherASCIIDeleteBoundary(t *testing.T) {
	raw := string([]byte{0x7f})
	matcher, err := ExtendedPatternMatcher(raw, EntireString)
	if err != nil {
		t.Fatal(err)
	}
	if !matcher(raw) {
		t.Fatal("0x7f literal did not match itself")
	}
}
