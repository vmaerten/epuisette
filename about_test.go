package main

import "testing"

// The page names a link; anything else must never reach the browser.
func TestOpenLinkRefusesAnUnknownName(t *testing.T) {
	for _, name := range []string{"", "https://example.com", "javascript:alert(1)"} {
		if err := openLink(name); err == nil {
			t.Errorf("openLink(%q) succeeded", name)
		}
	}
}
