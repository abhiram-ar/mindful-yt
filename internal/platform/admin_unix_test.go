//go:build unix

package platform

import (
	"errors"
	"os/exec"
	"slices"
	"testing"
)

func TestAdminCommandUsesSudoThenDoas(t *testing.T) {
	has := func(tools ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			if slices.Contains(tools, name) {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}
	}
	for _, c := range []struct {
		tools []string
		want  []string
	}{
		{[]string{"sudo", "doas"}, []string{"/usr/bin/sudo", "/opt/mindful-yt", "lock-me-in", "--write-hosts"}},
		{[]string{"doas"}, []string{"/usr/bin/doas", "/opt/mindful-yt", "lock-me-in", "--write-hosts"}},
	} {
		cmd, err := adminCommand(has(c.tools...), "/opt/mindful-yt", "lock-me-in", "--write-hosts")
		if err != nil {
			t.Fatal(err)
		}
		if got := cmd.(execCommand).Args; !slices.Equal(got, c.want) {
			t.Errorf("with %v: %q, want %q", c.tools, got, c.want)
		}
	}
	if _, err := adminCommand(has(), "/opt/mindful-yt"); err == nil || errors.Is(err, exec.ErrNotFound) {
		t.Errorf("with neither: %v", err)
	}
}
