package util

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadFilterFile parses a file into an array of non-empty, non-comment lines.
// If the file does not exist, then os.IsNotExist(err) == true
func LoadFilterFile(filePath string) ([]string, error) {
	var s []string
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return s, fmt.Errorf("failed to open filter file \"%s\": %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// ScanLines is default split function
	for scanner.Scan() {
		line := scanner.Text()
		// skip empty lines
		if line == "" {
			continue
		}
		// skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}
		s = append(s, line)
	}
	if err := scanner.Err(); err != nil {
		return s, fmt.Errorf("failed to parse diff filters file \"%s\": %v", filePath, err)
	}
	return s, nil
}
