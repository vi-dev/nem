package envx

import "strings"

var reservedNames = map[string]bool{
	"PATH":             true,
	"PROMPT_COMMAND":   true,
	"BASH_ENV":         true,
	"ENV":              true,
	"IFS":              true,
	"PS1":              true,
	"CDPATH":           true,
	"FPATH":            true,
	"MANPATH":          true,
	"MAILPATH":         true,
	"MODULE_PATH":      true,
	"FIGNORE":          true,
	"PSVAR":            true,
	"WATCH":            true,
	"GCONV_PATH":       true,
	"GLIBC_TUNABLES":   true,
	"LOCPATH":          true,
	"NLSPATH":          true,
	"GETCONF_DIR":      true,
	"HOSTALIASES":      true,
	"RESOLV_HOST_CONF": true,
}

func IsReserved(name string) bool {
	upper := strings.ToUpper(name)

	if reservedNames[upper] {
		return true
	}

	if strings.HasPrefix(upper, "NEM_") || strings.HasPrefix(upper, "LD_") || strings.HasPrefix(upper, "DYLD_") {
		return true
	}

	if strings.HasSuffix(upper, "_SET") {
		return true
	}

	return false
}
