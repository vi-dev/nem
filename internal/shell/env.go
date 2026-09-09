package shell

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/spec"
)

type Dialect int

const (
	Bash Dialect = iota

	Zsh

	Fish
)

const managedKeysVar = "NEM_MANAGED_KEYS"

func EnvScript(d Dialect, res envx.Result, getenv func(string) (string, bool)) (string, error) {
	switch d {
	case Bash, Zsh:
	case Fish:
		return "", errors.New("fish is not supported yet")
	default:
		return "", fmt.Errorf("unsupported shell dialect: %d", int(d))
	}

	newKeys := make(map[string]bool, len(res.Vars))
	newKeyList := make([]string, 0, len(res.Vars))

	var b strings.Builder
	for _, v := range res.Vars {
		if !spec.EnvNameRE.MatchString(v.Name) {
			continue
		}
		newKeys[v.Name] = true
		newKeyList = append(newKeyList, v.Name)
		writeSave(&b, v.Name, getenv)
		fmt.Fprintf(&b, "export %s=%s\n", v.Name, quote(v.Value))
	}

	priorList, _ := getenv(managedKeysVar)
	seen := make(map[string]bool)
	for k := range strings.FieldsSeq(priorList) {
		if !spec.EnvNameRE.MatchString(k) || seen[k] {
			continue
		}
		seen[k] = true
		if newKeys[k] {
			continue
		}
		writeRestore(&b, k, getenv)
	}

	writePath(&b, res.Path)
	writeLoaderPath(&b, res.LoaderVar, res.LoaderPath, getenv)

	fmt.Fprintf(&b, "export %s=%s\n", managedKeysVar, quote(strings.Join(newKeyList, " ")))

	return b.String(), nil
}

func writeSave(b *strings.Builder, name string, getenv func(string) (string, bool)) {
	savedSetName := "NEM_SAVED__" + name + "_SET"
	if _, ok := getenv(savedSetName); ok {
		return
	}

	savedName := "NEM_SAVED__" + name
	current, isSet := getenv(name)
	if isSet {
		fmt.Fprintf(b, "export %s=%s\n", savedName, quote(current))
		fmt.Fprintf(b, "export %s=%s\n", savedSetName, quote("1"))
		return
	}
	fmt.Fprintf(b, "export %s=%s\n", savedSetName, quote("0"))
}

func writeRestore(b *strings.Builder, name string, getenv func(string) (string, bool)) {
	savedName := "NEM_SAVED__" + name
	savedSetName := savedName + "_SET"

	if wasSet, _ := getenv(savedSetName); wasSet == "1" {
		saved, _ := getenv(savedName)
		fmt.Fprintf(b, "export %s=%s\n", name, quote(saved))
	} else {
		fmt.Fprintf(b, "unset %s\n", name)
	}
	fmt.Fprintf(b, "unset %s %s\n", savedName, savedSetName)
}

func writePath(b *strings.Builder, paths []string) {
	b.WriteString(`export NEM_ORIGINAL_PATH="${NEM_ORIGINAL_PATH-$PATH}"` + "\n")

	if len(paths) == 0 {
		b.WriteString(`export PATH="${NEM_ORIGINAL_PATH:-$PATH}"` + "\n")
		return
	}

	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = quote(p)
	}
	fmt.Fprintf(b, "export PATH=%s:\"${NEM_ORIGINAL_PATH:-$PATH}\"\n", strings.Join(quoted, ":"))
}

func writeLoaderPath(b *strings.Builder, name string, dirs []string, getenv func(string) (string, bool)) {
	if name == "" {
		return
	}
	setVal, managed := getenv("NEM_SAVED__" + name + "_SET")

	if len(dirs) == 0 {
		if managed {
			writeRestore(b, name, getenv)
		}
		return
	}

	origValue := ""
	origWasSet := false
	if managed {
		if setVal == "1" {
			origWasSet = true
			origValue, _ = getenv("NEM_SAVED__" + name)
		}
	} else {
		origValue, origWasSet = getenv(name)
		writeSave(b, name, getenv)
	}

	quoted := make([]string, len(dirs))
	for i, d := range dirs {
		quoted[i] = quote(d)
	}
	value := strings.Join(quoted, ":")
	if origWasSet {
		value += ":" + quote(origValue)
	}
	fmt.Fprintf(b, "export %s=%s\n", name, value)
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
