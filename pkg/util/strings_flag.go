package util

import (
	"flag"
	"strings"
)

type StringsFlagValue []string

func (s *StringsFlagValue) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *StringsFlagValue) String() string {
	return strings.Join(*s, ", ")
}

func StringsFlag(name string, value []string, usage string) *StringsFlagValue {
	var v StringsFlagValue
	if value != nil {
		v = append(v, value...)
	}
	flag.Var(&v, name, usage)
	return &v
}
