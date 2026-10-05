package ytdlp

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

const timeoutHelperArgument = "mixora-ytdlp-timeout-helper"

func TestRunCommandHonorsClientTimeout(t *testing.T) {
	client := New(WithBinaryPath(os.Args[0]), WithTimeout(25*time.Millisecond))
	started := time.Now()
	_, err := client.runCommand(context.Background(), "-test.run=TestYTDLPTimeoutHelper", timeoutHelperArgument)
	if !errors.Is(err, ErrExtractionFailed) {
		t.Fatalf("runCommand error = %v, want extraction error", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runCommand error = %v, want preserved deadline", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("runCommand ignored configured timeout: %s", elapsed)
	}
}

func TestYTDLPTimeoutHelper(t *testing.T) {
	for _, arg := range os.Args {
		if arg == timeoutHelperArgument {
			time.Sleep(time.Second)
			return
		}
	}
}
