package format

import "testing"

func TestCountLineSimple(t *testing.T) {
	got := CountLine(CountLineOptions{Count: 5})
	if got != "count: 5" {
		t.Fatalf("got %q", got)
	}
}

func TestCountLineAPILimit(t *testing.T) {
	got := CountLine(CountLineOptions{Count: 1000, APILimitHit: true})
	if got != "count: 1000+ (GitHub search API limit reached)" {
		t.Fatalf("got %q", got)
	}
}

func TestCountLineTotal(t *testing.T) {
	total := 100
	got := CountLine(CountLineOptions{Count: 30, TotalCount: &total})
	if got != "count: 30 of 100 total" {
		t.Fatalf("got %q", got)
	}
}
