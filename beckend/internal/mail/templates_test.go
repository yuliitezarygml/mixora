package mail

import (
	"strings"
	"testing"
)

func TestAccountActionLinksOpenClientPages(t *testing.T) {
	verification := VerificationMessage("http://127.0.0.1:5174/", "listener@example.test", "Listener", "verify token")
	if !strings.Contains(verification.TextBody, "http://127.0.0.1:5174/verify-email?token=verify+token") {
		t.Fatalf("verification link does not target the client page: %q", verification.TextBody)
	}

	reset := PasswordResetMessage("http://127.0.0.1:5174/", "listener@example.test", "Listener", "reset token")
	if !strings.Contains(reset.TextBody, "http://127.0.0.1:5174/reset-password?token=reset+token") {
		t.Fatalf("reset link does not target the client page: %q", reset.TextBody)
	}
}
