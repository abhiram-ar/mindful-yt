package human

import "testing"

func TestBytes(t *testing.T) {
	for n, want := range map[float64]string{
		512: "512 B", 2048: "2 KB", 12.3 * (1 << 20): "12.3 MB", 1.5 * (1 << 30): "1.5 GB",
	} {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%v) = %q, want %q", n, got, want)
		}
	}
}

func TestDuration(t *testing.T) {
	for s, want := range map[float64]string{0: "?", 19: "0:19", 634: "10:34", 3725: "1:02:05"} {
		if got := Duration(s); got != want {
			t.Errorf("Duration(%v) = %q, want %q", s, got, want)
		}
	}
}
