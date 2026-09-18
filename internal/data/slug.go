package data

import "strings"

// SlugifyName turns a display name into a stable id: lowercase alphanumerics
// separated by single dashes.
func SlugifyName(value string) string {
	var builder strings.Builder
	lastWasDash := false

	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}

		if builder.Len() > 0 && !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}
