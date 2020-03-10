package kube

import (
	yaml "gopkg.in/yaml.v2"
)

// filterYaml will deep copy m and remove the element at the yamlPath.
func filterYaml(m yaml.MapSlice, yamlPath ...string) yaml.MapSlice {
	var out yaml.MapSlice
	for _, item := range m {
		if f, ok := item.Key.(string); ok && f == yamlPath[0] {
			// path match found, skip element
			if len(yamlPath) == 1 {
				continue
			}

			// path match found, recurse into children
			if mm, ok := item.Value.(yaml.MapSlice); ok && len(yamlPath) > 1 {
				item = yaml.MapItem{
					Key:   item.Key,
					Value: filterYaml(mm, yamlPath[1:]...),
				}
			}
		}

		out = append(out, item)
	}
	return out
}

func filterEmpty(m yaml.MapSlice) yaml.MapSlice {
	var out yaml.MapSlice
	for _, item := range m {
		if value, ok := item.Value.(yaml.MapSlice); ok {
			// empty value, skip item
			if len(value) == 0 {
				continue
			}

			value = filterEmpty(value)

			// empty value after filtering, skip item
			if len(value) == 0 {
				continue
			}

			item = yaml.MapItem{
				Key:   item.Key,
				Value: value,
			}
		}

		out = append(out, item)
	}
	return out
}
