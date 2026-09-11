package handlers

import "testing"

func TestNormalizePremiumPhone(t *testing.T) {
	for _, tc := range []struct {
		input, want string
	}{
		{"+14155552671", "14155552671"},
		{"14155552671", "14155552671"},
		{"00447700900123", "447700900123"},
		{"447700900123", "447700900123"},
		{"+919876543210", "919876543210"},
		{"819012345678", "819012345678"},
		{"821012345678", "821012345678"},
		{"8613812345678", "8613812345678"},
		{"85291234567", "85291234567"},
		{"+5511987654321", "5511987654321"},
		{"+2348031234567", "2348031234567"},
		{"+61412345678", "61412345678"},
		{"+628123456789", "628123456789"},
		{"628123456789", "628123456789"},
		{"00628123456789", "628123456789"},
		{"08123456789", "628123456789"},
		{" +390212345678 ", "390212345678"},
		{"123456789012345", "123456789012345"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizePremiumPhone(tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("normalizePremiumPhone(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestNormalizePremiumPhoneRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{
		"", " ", "+", "00", "0", "1", "0123456789", "+08123456789", "000819012345678",
		"++14155552671", "00+14155552671", "1415abc2671", "+1-415-555-2671",
		"+1 4155552671", "１２３４５６７８９", "1234567890123456", "0812345678901234",
		"14155552671@s.whatsapp.net", "123@g.us",
	} {
		t.Run(input, func(t *testing.T) {
			if got, err := normalizePremiumPhone(input); err == nil || got != "" {
				t.Fatalf("invalid input %q accepted as %q", input, got)
			}
		})
	}
}
