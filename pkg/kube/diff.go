// Copyright 2019 GM Cruise LLC
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

package kube

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	yaml "gopkg.in/yaml.v2"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// filterYamlField will traverse m and filter out all items with key matching
// field. Will descend down into compound slice values if recurse is true.
func filterYamlField(m yaml.MapSlice, field string, recurse bool) yaml.MapSlice {
	var out yaml.MapSlice
	for _, item := range m {
		// Skip items that match field.
		if f, ok := item.Key.(string); ok && f == field {
			continue
		}

		if mm, ok := item.Value.(yaml.MapSlice); ok && recurse {
			item = yaml.MapItem{
				Key:   item.Key,
				Value: filterYamlField(mm, field, recurse),
			}
		}

		out = append(out, item)
	}
	return out
}

// renderObj renders obj into JSON or YAML (if renderYaml is true), while
// stripping the output off of secrets, fields that are set by API Server
// (SelfLink, UID, etc).
// Also sets defaults.
func renderObj(obj runtime.Object, gvk *schema.GroupVersionKind, renderYaml bool) (string, error) {
	// Make sure secrets aren't leaked into logs/console.
	if s, ok := obj.(*corev1.Secret); ok {
		newSecret := s.DeepCopy()
		for k := range newSecret.Data {
			newSecret.Data[k] = nil
		}
		for k := range newSecret.StringData {
			newSecret.StringData[k] = "<redacted>"
		}
		obj = newSecret
	}

	Scheme.Default(obj)

	mObj, ok := obj.(metav1.Object)
	if !ok {
		return "", fmt.Errorf("object does not implement metav1.Object: %v", obj)
	}

	// Filter fields that would always differ.
	mObj.SetSelfLink("")
	mObj.SetUID("")
	mObj.SetGeneration(0)

	jsonBytes, err := json.MarshalIndent(obj, "", "\t")
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %v", err)
	}

	if !renderYaml {
		return string(jsonBytes), nil
	}

	var yamlMap yaml.MapSlice
	if err := yaml.Unmarshal(jsonBytes, &yamlMap); err != nil {
		return "", fmt.Errorf("failed to unmarshal to YAML: %v", err)
	}

	// If kind and apiVersion are not already set, recover them from gvk.
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if gvk != nil && kind == "" {
		apiVersion := gvk.GroupVersion().String()
		yamlMap = append(
			[]yaml.MapItem{
				{Key: "kind", Value: gvk.Kind},
				{Key: "apiVersion", Value: apiVersion},
			},
			yamlMap...)
	}

	yamlMap = filterYamlField(yamlMap, "creationTimestamp", true)
	yamlMap = filterYamlField(yamlMap, "status", false)
	yamlMap = filterYamlField(yamlMap, "serviceAccount", false)

	yamlBytes, err := yaml.Marshal(yamlMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML map: %v", err)
	}

	return string(yamlBytes), nil
}

func maybeNamespaced(name, ns string) string {
	if ns != "" {
		return ns + "/" + name
	}
	return name
}

func maybeCore(group string) string {
	if group == "" {
		return ".v1"
	}
	return "." + group
}

// printUnifgiedDiff prints unified diff of live against head. Uses gvk and
// name to prettify the diff.
// If live is nil, just prints the right side.
func printUnifiedDiff(w io.Writer, live, head runtime.Object, gvk schema.GroupVersionKind, name string) error {
	fullName := fmt.Sprintf("%s%s `%s'", strings.ToLower(gvk.Kind), maybeCore(gvk.Group), name)

	var left string
	if live != nil {
		var err error
		left, err = renderObj(live, nil, true)
		if err != nil {
			return fmt.Errorf("failed to render :live object for %s: %v", fullName, err)
		}
	}

	right, _ := renderObj(head, &gvk, true)

	fmt.Fprintf(w, "\n*** %s ***\n", fullName)

	err := difflib.WriteUnifiedDiff(w, difflib.UnifiedDiff{
		A:        difflib.SplitLines(left),
		B:        difflib.SplitLines(right),
		FromFile: "live",
		ToFile:   "head",
		Context:  5,
		Eol:      "\n",
	})
	if err != nil {
		return fmt.Errorf("failed to print diff for %s: %v", fullName, err)
	}
	return nil
}
