package fetch

import "fmt"

type ArtifactNotFoundError struct{ Name, Version, Platform, Missing, URL string }

func (e *ArtifactNotFoundError) Error() string {
	msg := fmt.Sprintf("artifact for %s@%s (%s) not found: missing %s", e.Name, e.Version, e.Platform, e.Missing)
	if e.URL != "" {
		msg += " (" + e.URL + ")"
	}
	return msg
}

type ChecksumMismatchError struct{ Name, Version, Platform, Got, Want string }

func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch for %s@%s (%s): got %s, want %s", e.Name, e.Version, e.Platform, e.Got, e.Want)
}
