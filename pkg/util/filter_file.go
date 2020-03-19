// Copyright 2020 Cruise LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
