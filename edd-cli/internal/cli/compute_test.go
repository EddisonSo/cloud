package cli

import (
	"bytes"
	"strings"
	"testing"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func TestContainerTable(t *testing.T) {
	var buf bytes.Buffer
	containerTable(&buf, []eddsdk.Container{
		{ID: "abc", Name: "web", Status: "running", InstanceType: "nano", PullPolicy: "Always"},
	})
	out := buf.String()
	for _, want := range []string{"ID", "abc", "web", "running", "nano", "Always"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}
