package explore

import (
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/docs"
)

func TestSearchGuideFindsTheRightSection(t *testing.T) {
	got := searchGuide(docs.Guide, "tailscale QR code phone")
	if !strings.HasPrefix(got, "## On the phone") {
		t.Fatalf("%.200s", got)
	}
	if heads := searchGuide(docs.Guide, ""); !strings.Contains(heads, "## Chat") || strings.Contains(heads, "Confirmar e fazer") {
		t.Fatalf("headings: %s", heads)
	}
	if got := searchGuide(docs.Guide, "xyzzy"); !strings.HasPrefix(got, "Nothing in the guide matches") {
		t.Fatalf("%.100s", got)
	}
}
