package api

import "testing"

func TestStaticCacheControl(t *testing.T) {
	if got := staticCacheControl("/assets/index-Dzh-KFvz.js"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset cache policy = %q", got)
	}
	if got := staticCacheControl("/index.html"); got != "no-cache" {
		t.Fatalf("index cache policy = %q", got)
	}
}
