package envx

func Expand(s string, lookup func(name string) (string, bool)) string {
	var result []rune
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		if runes[i] != '$' {
			result = append(result, runes[i])
			continue
		}

		if i+1 >= len(runes) {

			result = append(result, '$')
			continue
		}

		next := runes[i+1]

		if next == '$' {
			result = append(result, '$')
			i++
			continue
		}

		if next == '{' {

			closeIdx := -1
			for j := i + 2; j < len(runes); j++ {
				if runes[j] == '}' {
					closeIdx = j
					break
				}
			}

			if closeIdx == -1 {

				result = append(result, runes[i:]...)
				break
			}

			varName := string(runes[i+2 : closeIdx])

			if !isValidName(varName) {

				result = append(result, '$')
				result = append(result, '{')
				result = append(result, []rune(varName)...)
				result = append(result, '}')
				i = closeIdx
				continue
			}

			value, ok := lookup(varName)
			if !ok {
				value = ""
			}
			result = append(result, []rune(value)...)
			i = closeIdx
			continue
		}

		if isNameStart(next) {
			nameStart := i + 1
			nameEnd := nameStart

			for nameEnd < len(runes) && isNameChar(runes[nameEnd]) {
				nameEnd++
			}

			varName := string(runes[nameStart:nameEnd])
			value, ok := lookup(varName)
			if !ok {
				value = ""
			}
			result = append(result, []rune(value)...)
			i = nameEnd - 1
			continue
		}

		result = append(result, '$')
	}

	return string(result)
}

func isNameStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isNameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func isValidName(s string) bool {
	if len(s) == 0 {
		return false
	}
	if !isNameStart(rune(s[0])) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isNameChar(rune(s[i])) {
			return false
		}
	}
	return true
}
