package app

import "testing"

func TestIsValidEmail(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"foo", false},
		{"foo@", false},
		{"@bar.com", false},
		{"foo@bar", false},
		{"foo@example.com", false},
		{"FOO@EXAMPLE.COM", false},
		{"foo@example.org", false},
		{"foo@test.com", false},
		{"foo@localhost", false},
		{"a@b.co", true},
		{"user.dev+research@univ.edu", true},
		{"first.last@sub.domain.example", true}, // last TLD .example — still passes regex; placeholder check only blocks exact hosts
		{"user@uni-bonn.de", true},
	}
	for _, tc := range cases {
		got := IsValidEmail(tc.in)
		if got != tc.want {
			t.Errorf("IsValidEmail(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
