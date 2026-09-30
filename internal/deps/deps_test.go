package deps

import "testing"

func TestExpectedSum(t *testing.T) {
	sums := "aa11  yt-dlp\nBB22  yt-dlp.exe\ncc33  yt-dlp_x86.exe\n"
	if got := expectedSum(sums, "yt-dlp.exe"); got != "bb22" {
		t.Errorf("got %q", got)
	}
	if got := expectedSum(sums, "missing.exe"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestJSRuntimeVersionRules(t *testing.T) {
	node, deno := jsRuntimes[0], jsRuntimes[1]
	for _, c := range []struct {
		rt     jsRuntime
		output string
		want   string
	}{
		{node, "v24.17.0\n", "node 24.17"},
		{node, "v22.0.0\n", "node 22.0"},
		{node, "v20.11.1\n", ""}, // older than yt-dlp's minimum
		{deno, "deno 2.9.6 (stable, release, x86_64-pc-windows-msvc)\n", "deno 2.9"},
		{deno, "deno 2.2.0\n", ""},
		{node, "not a version", ""},
	} {
		if got, _ := c.rt.check(c.output); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.rt.name, c.output, got, c.want)
		}
	}
}
