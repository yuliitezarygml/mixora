package gorse

import "testing"

func TestFeedbackWeightsPreserveStrongerIntent(t *testing.T) {
	t.Parallel()
	if feedbackWeight("like") <= feedbackWeight("listen_30s") {
		t.Fatal("like should be stronger than a partial listen")
	}
	if feedbackWeight("complete") <= feedbackWeight("play") {
		t.Fatal("completion should be stronger than play")
	}
	if feedbackWeight("impression") >= feedbackWeight("play") {
		t.Fatal("impression should be weaker than play")
	}
}
