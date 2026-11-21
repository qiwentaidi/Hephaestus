package ai

import (
	"strconv"
	"testing"
)

func TestParseOrchestrationSourceTaskIDs(t *testing.T) {
	tooMany := make([]string, 0, 9)
	for i := 1; i <= 9; i++ {
		tooMany = append(tooMany, strconv.Itoa(i))
	}
	cases := []struct {
		name string
		in   []string
		want []int64
		bad  bool
	}{
		{name: "empty", in: nil, want: nil},
		{name: "keeps order and trims", in: []string{" 12 ", "7"}, want: []int64{12, 7}},
		{name: "skips blank picks", in: []string{"", " 3"}, want: []int64{3}},
		{name: "duplicate", in: []string{"5", "5"}, bad: true},
		{name: "not a number", in: []string{"abc"}, bad: true},
		{name: "zero", in: []string{"0"}, bad: true},
		{name: "negative", in: []string{"-1"}, bad: true},
		{name: "over the inheritance cap", in: tooMany, bad: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseOrchestrationSourceTaskIDs(test.in)
			if test.bad {
				if err == nil {
					t.Fatalf("expected rejection, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("got %v, want %v", got, test.want)
				}
			}
		})
	}
}
