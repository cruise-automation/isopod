package kpath

import (
	"errors"
	"strings"
)

type kpath struct {
	Part string // current key part
	Path string // remaining path
	More bool   // there is more path to parse
}

// Split parses a kpath string into an array of parts.
// Kpath parts are delimited either by a period (ex: partA.partB) or brackets and quotes: (ex: partA["partB"]).
// To index an array, use brackets with an numeric index (ex: partA[0]).
// All delimiters may be used in combination (ex: partA.partB["partC"].partD[1]).
// Period delimiting is preferred when indexing a map, unless the part contains a period, in which case brackets and
// quotes should be used.
func Split(path string) ([]string, error) {
	var s []string
	for {
		r, err := parse(path)
		if err != nil {
			return s, err
		}
		s = append(s, r.Part)
		path = r.Path
		if !r.More {
			break
		}
	}
	return s, nil
}

// parse extracts the first part in a kpath string, returning the part, the remaining path, and whether the remaining
// path is expected to have more parts.
// If more=true and path="", the next parse call should error (usually because of a trailing delimiter).
func parse(path string) (kpath, error) {
	var r kpath
	if path == "" {
		return r, errors.New("empty path")
	}
	if path[0] == '[' {
		var i int
		if len(path) < 2 {
			return r, errors.New("unclosed array index in path")
		}
		if path[1] == '"' {
			// explicit string map index
			i = strings.Index(path, "\"]")
			if i < 0 {
				return r, errors.New("unclosed map index in path")
			}
			r.Part = path[2:i]
			i += 2
		} else {
			// array index
			i = strings.IndexRune(path, ']')
			if i < 0 {
				return r, errors.New("unclosed array index in path")
			}
			r.Part = path[1:i]
			i++
		}
		if len(path) > i {
			r.More = true
			if path[i] == '.' {
				// exlude delimiter
				r.Path = path[i+1:]
			} else {
				// include delimiter
				r.Path = path[i:]
			}
		}
		return r, nil
	}
	// implicit string map index
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			// exlude delimiter
			r.Part = path[:i]
			r.Path = path[i+1:]
			r.More = true
			return r, nil
		}
		if path[i] == '[' {
			// include delimiter
			r.Part = path[:i]
			r.Path = path[i:]
			r.More = true
			return r, nil
		}
	}
	// entire path is the last part
	r.Part = path
	return r, nil
}
