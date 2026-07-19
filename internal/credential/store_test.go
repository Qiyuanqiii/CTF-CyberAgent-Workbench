package credential

import (
	"strings"
	"testing"
)

func TestValidSecretHonorsCredentialManagerBlobLimit(t *testing.T) {
	if !ValidSecret(strings.Repeat("a", MaxSecretBytes)) {
		t.Fatal("maximum-size credential was rejected")
	}
	if ValidSecret(strings.Repeat("a", MaxSecretBytes+1)) {
		t.Fatal("oversized credential was accepted")
	}
	if ValidSecret(" leading") || ValidSecret("embedded secret") || ValidSecret("line\nbreak") {
		t.Fatal("non-normalized credential was accepted")
	}
}
