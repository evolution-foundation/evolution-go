package utils

import (
	"strings"
	"testing"
)

func TestCreateJID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		// Basic phone numbers
		{
			name:     "Simple US number",
			input:    "15551234567",
			expected: "+15551234567@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Number with spaces and parentheses",
			input:    "+1 (555) 123-4567",
			expected: "+15551234567@s.whatsapp.net",
			hasError: false,
		},

		// Brazilian numbers (55)
		{
			name:     "BR mobile number with 9 prefix - should keep 9",
			input:    "5531987654321",
			expected: "+5531987654321@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "BR landline number - should keep as is",
			input:    "5531123456789",
			expected: "+5531123456789@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "BR number with DDD < 31 - should keep as is",
			input:    "5521987654321",
			expected: "+5521987654321@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "BR number with first digit < 7 - should keep as is",
			input:    "5531687654321",
			expected: "+5531687654321@s.whatsapp.net",
			hasError: false,
		},

		// Portuguese numbers (351) - should not be treated as Brazilian
		{
			name:     "Portugal number - should not apply BR formatting",
			input:    "351932933862",
			expected: "+351932933862@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Portugal number with + - should not apply BR formatting",
			input:    "+351932933862",
			expected: "+351932933862@s.whatsapp.net",
			hasError: false,
		},

		// Mexican numbers (52)
		{
			name:     "MX number with extra digit - should remove",
			input:    "5215551234567",
			expected: "+52551234567@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "MX number without extra digit",
			input:    "525551234567",
			expected: "+525551234567@s.whatsapp.net",
			hasError: false,
		},

		// Argentine numbers (54)
		{
			name:     "AR number with extra digit - should remove",
			input:    "5411123456789",
			expected: "+541123456789@s.whatsapp.net",
			hasError: false,
		},

		// Group IDs
		{
			name:     "Group ID with hyphen",
			input:    "120363123456789012@g.us",
			expected: "120363123456789012@g.us",
			hasError: false,
		},
		{
			name:     "Group ID by length",
			input:    "120363123456789012345",
			expected: "120363123456789012345@g.us",
			hasError: false,
		},
		{
			name:     "Group ID with hyphen and extra chars",
			input:    "120363-123456789012-345abc",
			expected: "120363-123456789012-345@g.us",
			hasError: false,
		},

		// Already formatted JIDs
		{
			name:     "Already formatted user JID",
			input:    "15551234567@s.whatsapp.net",
			expected: "15551234567@s.whatsapp.net",
			hasError: false,
		},
		{
			name:     "Already formatted group JID",
			input:    "120363123456789012@g.us",
			expected: "120363123456789012@g.us",
			hasError: false,
		},
		{
			name:     "Broadcast JID",
			input:    "status@broadcast",
			expected: "status@broadcast",
			hasError: false,
		},
		{
			name:     "LID JID",
			input:    "12345@lid",
			expected: "12345@lid",
			hasError: false,
		},

		// Error cases
		{
			name:     "Empty string",
			input:    "",
			expected: "",
			hasError: true,
		},
		{
			name:     "Only special characters",
			input:    "+-()@#$%",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CreateJID(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("For input %q, expected %q, but got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestFormatMXOrARNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "MX number with 13 digits",
			input:    "5215551234567",
			expected: "52551234567",
		},
		{
			name:     "MX number with 12 digits",
			input:    "525551234567",
			expected: "525551234567",
		},
		{
			name:     "AR number with 13 digits",
			input:    "5411123456789",
			expected: "541123456789",
		},
		{
			name:     "Non-MX/AR number",
			input:    "5511987654321",
			expected: "5511987654321",
		},
		{
			name:     "Short number",
			input:    "52",
			expected: "52",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMXOrARNumber(tt.input)
			if result != tt.expected {
				t.Errorf("For input %q, expected %q, but got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestFormatBRNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "BR mobile number with DDD >= 31 and 9 prefix",
			input:    "5531987654321",
			expected: "5531987654321",
		},
		{
			name:     "BR number with DDD < 31",
			input:    "5521987654321",
			expected: "5521987654321",
		},
		{
			name:     "BR number with first digit < 7",
			input:    "5531687654321",
			expected: "5531687654321",
		},
		{
			name:     "Non-BR number",
			input:    "15551234567",
			expected: "15551234567",
		},
		{
			name:     "BR number with wrong length",
			input:    "551198765432",
			expected: "551198765432",
		},
		{
			name:     "BR landline number",
			input:    "5531123456789",
			expected: "5531123456789",
		},
		{
			name:     "Portugal number starting with 55 - should not be treated as BR",
			input:    "351932933862",
			expected: "351932933862",
		},
		{
			name:     "Number starting with 55 but invalid DDD - should not be treated as BR",
			input:    "5509987654321",
			expected: "5509987654321",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBRNumber(tt.input)
			if result != tt.expected {
				t.Errorf("For input %q, expected %q, but got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestParseJID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectOk  bool
		expectJID string
	}{
		{
			name:      "Valid phone number",
			input:     "15551234567",
			expectOk:  true,
			expectJID: "+15551234567@s.whatsapp.net",
		},
		{
			name:      "Valid BR number",
			input:     "5531987654321",
			expectOk:  true,
			expectJID: "+5531987654321@s.whatsapp.net",
		},
		{
			name:      "Valid group ID",
			input:     "120363123456789012@g.us",
			expectOk:  true,
			expectJID: "120363123456789012@g.us",
		},
		{
			name:      "Empty string",
			input:     "",
			expectOk:  false,
			expectJID: "@s.whatsapp.net",
		},
		{
			name:      "Invalid format",
			input:     "+-()@#$%",
			expectOk:  false,
			expectJID: "@s.whatsapp.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jid, ok := ParseJID(tt.input)

			if ok != tt.expectOk {
				t.Errorf("For input %q, expected ok=%v, but got ok=%v", tt.input, tt.expectOk, ok)
			}

			if tt.expectOk && jid.String() != tt.expectJID {
				t.Errorf("For input %q, expected JID %q, but got %q", tt.input, tt.expectJID, jid.String())
			}
		})
	}
}

// TestCanonicalJID locks in the fix for the silent typing-indicator / receipt
// drop: ParseJID (via CreateJID) prefixes phone numbers with "+", which is
// tolerated by message sending (usync normalizes it) but breaks RAW nodes
// (chatstate, read receipts, reactions). CanonicalJID must strip that "+" while
// leaving already-canonical JIDs (LID, group, digits-only) untouched.
func TestCanonicalJID(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedJID string // canonical JID expected after ParseJID -> CanonicalJID
	}{
		{
			name:        "BR phone strips the leading +",
			input:       "554187083284",
			expectedJID: "554187083284@s.whatsapp.net",
		},
		{
			name:        "US phone strips the leading +",
			input:       "15551234567",
			expectedJID: "15551234567@s.whatsapp.net",
		},
		{
			name:        "Already-canonical JID is unchanged",
			input:       "554187083284@s.whatsapp.net",
			expectedJID: "554187083284@s.whatsapp.net",
		},
		{
			name:        "LID JID is left untouched",
			input:       "15883309207561@lid",
			expectedJID: "15883309207561@lid",
		},
		{
			name:        "Group JID is left untouched",
			input:       "120363123456789012@g.us",
			expectedJID: "120363123456789012@g.us",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jid, ok := ParseJID(tt.input)
			if !ok {
				t.Fatalf("ParseJID(%q) failed", tt.input)
			}

			canonical := CanonicalJID(jid)

			if got := canonical.String(); got != tt.expectedJID {
				t.Errorf("CanonicalJID for input %q = %q, want %q", tt.input, got, tt.expectedJID)
			}

			if strings.HasPrefix(canonical.User, "+") {
				t.Errorf("CanonicalJID for input %q still has '+' in User: %q", tt.input, canonical.User)
			}
		})
	}
}
