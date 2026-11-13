package proxmox

import "testing"

func TestValidateBaseURL(t *testing.T) {
	valid := []string{
		"https://172.30.0.10:8006",
		"https://pve.example.internal:8006/",
	}
	for _, value := range valid {
		if _, err := ValidateBaseURL(value); err != nil {
			t.Errorf("expected %q to be valid: %v", value, err)
		}
	}
	invalid := []string{
		"http://172.30.0.10:8006",
		"https://172.30.0.10",
		"https://user:pass@172.30.0.10:8006",
		"https://172.30.0.10:8006/api2/json",
		"https://172.30.0.10:8006/?next=http://example.com",
	}
	for _, value := range invalid {
		if _, err := ValidateBaseURL(value); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}
