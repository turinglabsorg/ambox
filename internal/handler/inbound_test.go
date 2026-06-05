package handler

import "testing"

func TestNormalizeRecipientEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercases and trims direct recipient",
			input: "  MTRO@AMBOX.DEV  ",
			want:  "mtro@ambox.dev",
		},
		{
			name:  "strips plus tag from local part",
			input: "mtro+autohost-123@ambox.dev",
			want:  "mtro@ambox.dev",
		},
		{
			name:  "keeps plus as first local character untouched",
			input: "+invalid@ambox.dev",
			want:  "+invalid@ambox.dev",
		},
		{
			name:  "returns malformed address normalized only",
			input: "  NOT-AN-EMAIL  ",
			want:  "not-an-email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRecipientEmail(tt.input); got != tt.want {
				t.Fatalf("normalizeRecipientEmail(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
