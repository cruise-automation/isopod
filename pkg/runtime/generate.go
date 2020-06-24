// Copyright 2020 GM Cruise LLC
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

package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/cruise-automation/isopod/pkg/kube"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const indentString = "    "

var out = func(format string, a ...interface{}) { fmt.Printf(format, a...) }

func Generate(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	var yamlOrJSONFile []byte
	if fi.IsDir() {
		filePaths, err := filepath.Glob(filepath.Join(path, "*"))
		if err != nil {
			return err
		}
		r := regexp.MustCompile(`.(json|yaml|yml)$`)
		var files [][]byte
		for _, path := range filePaths {
			if r.MatchString(path) {
				file, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				files = append(files, file)
			}
		}
		yamlOrJSONFile = bytes.Join(files, []byte(`---`))
	} else {
		yamlOrJSONFile, err = ioutil.ReadFile(path)
		if err != nil {
			return err
		}
	}

	yamlsOrJSONs := bytes.Split(yamlOrJSONFile, []byte(`---`))
	a := newAddonFile()

	decode := serializer.NewCodecFactory(kube.Scheme).UniversalDeserializer().Decode

	for _, yamlOrJSON := range yamlsOrJSONs {
		if len(bytes.TrimSpace(yamlOrJSON)) == 0 {
			continue
		}
		obj, _, err := decode(yamlOrJSON, nil, nil)
		if err == nil {
			a.addObject(obj)
			continue
		}
		if !k8sruntime.IsNotRegisteredError(err) {
			return err
		}
		j, err := yaml.ToJSON(yamlOrJSON)
		if err != nil {
			return fmt.Errorf("couldn't extract json from input: %w", err)
		}
		var u unstructured.Unstructured
		if err := u.UnmarshalJSON(j); err != nil {
			return fmt.Errorf("couldn't unmarshal custom resource: %w", err)
		}
		a.addObject(u)
	}
	starlark := a.gen()
	out("%s", starlark)
	return nil
}

type addonFile struct {
	// pkgMap is a key value pair of the exact proto import path and a shorthand alias
	pkgMap map[string]string
	// pkgs is a list of proto import paths, that matches the keys of pkgMap. This list is used to persist the order
	// across multiple runs
	pkgs []string
	// objects contains a list of all kubernetes objects parsed from the input
	objects []interface{}
	// metaData collects metadata for all objects that are needed for the delete statements. This metaData would be
	// hard to acquire otherwise
	metaData []metaData
	// currentIndex points to current item in metaData and can be used in generation functions to add to metaData
	currentIndex int
}

type metaData struct {
	name      string
	namespace string
	group     string
	kind      string
}

func newAddonFile() *addonFile {
	return &addonFile{
		pkgMap: map[string]string{},
	}
}

func (a *addonFile) addObject(object interface{}) {
	a.metaData = append(a.metaData, metaData{})
	a.objects = append(a.objects, object)
}

func (a *addonFile) gen() []byte {
	buf := bytes.NewBuffer([]byte{})

	// vim tag for github and vim to render the file nicely
	buf.WriteString("# vim: set syntax=python:\n\n")

	// has to be generated before imports, because packages are filled as walking through the objects
	install := a.genInstall()

	// imports
	buf.Write(a.genImports())

	// install
	buf.Write(install)

	// remove
	kubeDelete := a.genKubeDeleteWithIndent(1)
	if len(kubeDelete) > 0 {
		buf.WriteString("\ndef remove(ctx):\n")
		buf.Write(kubeDelete)
	}

	return buf.Bytes()
}

func (a *addonFile) addPkg(pkg string) string {
	pkg = strings.ReplaceAll(pkg, "/", ".")
	pkg = strings.ReplaceAll(pkg, "-", "_")
	elems := strings.Split(pkg, ".")
	// crate alias of last two package elements
	if len(elems) > 1 {
		elems = elems[len(elems)-2:]
	}
	alias := strings.Join(elems, "")
	if _, ok := a.pkgMap[pkg]; !ok {
		a.pkgMap[pkg] = alias
		// add package also to list for consistency
		a.pkgs = append(a.pkgs, pkg)
	}
	return alias
}

func (a *addonFile) genImports() []byte {
	imports := bytes.NewBuffer([]byte{})
	for _, pkg := range a.pkgs {
		imports.WriteString(fmt.Sprintf("%s = proto.package(\"%s\")\n", a.pkgMap[pkg], pkg))
	}
	if len(a.pkgs) > 0 {
		imports.Write([]byte("\n"))
	}
	return imports.Bytes()
}

func (a *addonFile) genInstall() []byte {
	buf := bytes.NewBuffer([]byte{})
	buf.WriteString("def install(ctx):\n")
	for i, object := range a.objects {
		switch o := object.(type) {
		case k8sruntime.Object:
			a.writeKubePutWithIndent(buf, o, 1)
		case unstructured.Unstructured:
			a.writeKubePutYAMLWithIndent(buf, o, 1)
		}
		if i != len(a.objects)-1 {
			buf.WriteString("\n")
		}
		a.currentIndex++
	}
	return buf.Bytes()
}

func (a *addonFile) writeKubePutWithIndent(kubePut *bytes.Buffer, object k8sruntime.Object, indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	indent2 := bytes.Repeat([]byte(indentString), indent+1)
	data := a.genDataWithIndent(reflect.ValueOf(object), indent+2)

	group := object.GetObjectKind().GroupVersionKind().Group
	kind := object.GetObjectKind().GroupVersionKind().Kind

	a.metaData[a.currentIndex].group = group
	a.metaData[a.currentIndex].kind = kind

	kubePut.Write(indent1)
	kubePut.WriteString("kube.put(\n")

	// name
	kubePut.Write(indent2)
	kubePut.WriteString("name=\"" + a.metaData[a.currentIndex].name + "\",\n")

	// namespace
	if a.metaData[a.currentIndex].namespace != "" {
		kubePut.Write(indent2)
		kubePut.WriteString("namespace=\"" + a.metaData[a.currentIndex].namespace + "\",\n")
	}

	// api_group
	if group != "" {
		kubePut.Write(indent2)
		kubePut.WriteString("api_group=\"" + group + "\",\n")
	}

	// data
	kubePut.Write(indent2)
	kubePut.WriteString("data=[")
	kubePut.Write(data)
	kubePut.WriteString("]\n")

	kubePut.Write(indent1)
	kubePut.WriteString(")\n")

	return kubePut.Bytes()
}

func (a *addonFile) writeKubePutYAMLWithIndent(kubePutYAML *bytes.Buffer, c unstructured.Unstructured, indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	indent2 := bytes.Repeat([]byte(indentString), indent+1)
	name := c.GetName()
	namespace := c.GetNamespace()
	group := c.GroupVersionKind().Group
	kind := c.GroupVersionKind().Kind

	a.metaData[a.currentIndex] = metaData{
		name:      name,
		namespace: namespace,
		group:     group,
		kind:      kind,
	}

	kubePutYAML.Write(indent1)
	kubePutYAML.WriteString("data=")
	kubePutYAML.Write(a.genStarlarkStructWithIndent(c.Object, indent))
	kubePutYAML.WriteString("\n")

	kubePutYAML.Write(indent1)
	kubePutYAML.WriteString("kube.put_yaml(\n")

	// name
	kubePutYAML.Write(indent2)
	kubePutYAML.WriteString("name=\"" + name + "\",\n")

	// namespace
	if namespace != "" {
		kubePutYAML.Write(indent2)
		kubePutYAML.WriteString("namespace=\"" + namespace + "\",\n")
	}

	// data
	kubePutYAML.Write(indent2)
	kubePutYAML.WriteString(`data=[`)
	kubePutYAML.WriteString("data.to_json()")
	kubePutYAML.WriteString(`]` + "\n")

	kubePutYAML.Write(indent1)
	kubePutYAML.WriteString(")\n")
	return kubePutYAML.Bytes()
}

func (a *addonFile) genKubeDeleteWithIndent(indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	kubeDelete := bytes.NewBuffer([]byte{})
	for _, object := range a.metaData {
		kubeDelete.Write(indent1)
		kubeDelete.WriteString("kube.delete(")
		kubeDelete.WriteString(strings.ToLower(object.kind))
		kubeDelete.WriteString("=\"")
		if object.namespace != "" {
			kubeDelete.WriteString(object.namespace)
			kubeDelete.WriteString("/")
		}
		kubeDelete.WriteString(object.name)
		kubeDelete.WriteString("\"")
		if object.group != "" {
			kubeDelete.WriteString(fmt.Sprintf(", api_group=\"%s\"", object.group))
		}
		kubeDelete.WriteString(")\n")
	}
	return kubeDelete.Bytes()
}

func (a *addonFile) genDataWithIndent(v reflect.Value, indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	indentTopLevel := indent1
	if indent > 0 {
		indentTopLevel = bytes.Repeat([]byte(indentString), indent-1)
	}

	b := bytes.NewBuffer([]byte{})

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct && v.Kind() != reflect.Map {
		j, _ := json.Marshal(v.Interface())
		if bytes.Equal([]byte("true"), j) || bytes.Equal([]byte("false"), j) {
			// because Python's boolean is capitalized
			return bytes.Title(j)
		}
		return j
	}

	if v.Kind() == reflect.Map {
		b.WriteString("{\n")

		// order maps for reproducability
		var stringKeys []string
		keyValues := v.MapKeys()
		keyValueMap := map[string]reflect.Value{}
		for _, keyValue := range keyValues {
			stringKey := fmt.Sprintf("%v", keyValue)
			stringKeys = append(stringKeys, stringKey)
			keyValueMap[stringKey] = keyValue
		}
		sort.Strings(stringKeys)

		for i, key := range stringKeys {
			keyValue := keyValueMap[key]
			b.Write(indent1)
			mapKey := a.genDataWithIndent(keyValue, indent+1)
			b.Write(mapKey)
			b.WriteString(": ")
			mapValue := a.genDataWithIndent(v.MapIndex(keyValue), indent+1)
			b.Write(mapValue)
			if i != v.Len()-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.Write(indentTopLevel)
		b.WriteString("}")
		return b.Bytes()
	}

	t := v.Type()
	name, pkgPath := t.Name(), t.PkgPath()
	alias := a.addPkg(pkgPath)
	b.WriteString(alias + "." + name + "(\n")

	for i := 0; i < v.NumField(); i++ {
		vf := v.Field(i)
		if !vf.IsZero() {
			protoTag := t.Field(i).Tag.Get("protobuf")
			r := regexp.MustCompile(`name=([\w\d]+)`)
			groups := r.FindStringSubmatch(protoTag)
			var protoName string
			if len(groups) == 2 {
				protoName = groups[1]
			}

			// add name and namespace to metadata of object
			if t == reflect.TypeOf(v1.ObjectMeta{}) {
				switch protoName {
				case "name":
					a.metaData[a.currentIndex].name = vf.String()
				case "namespace":
					a.metaData[a.currentIndex].namespace = vf.String()
				}
			}

			if protoName == "" || protoName == "apiGroup" {
				continue
			}
			// add actual object
			b.Write(indent1)
			b.WriteString(protoName)
			b.WriteString("=")

			if vf.Kind() == reflect.Slice && vf.Type() != reflect.TypeOf([]byte(nil)) {
				b.WriteString("[")
				for i := 0; i < vf.Len(); i++ {
					if i != 0 {
						b.Write(indent1)
						if vf.Index(i).Kind() != reflect.Ptr && vf.Index(i).Kind() != reflect.Struct {
							b.WriteString(indentString)
						}
					}
					d := a.genDataWithIndent(vf.Index(i), indent+1)
					b.Write(d)
					if i != vf.Len()-1 {
						b.WriteString(",\n")
					}
				}
				b.WriteString("]")
			} else {
				d := a.genDataWithIndent(vf, indent+1)
				b.Write(d)
			}

			if i != v.NumField()-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
	}
	b.Write(indentTopLevel)
	b.WriteString(")")
	return b.Bytes()
}

func (a *addonFile) genStarlarkStructWithIndent(object interface{}, indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	indent2 := bytes.Repeat([]byte(indentString), indent+1)
	b := bytes.NewBuffer([]byte{})

	if reflect.ValueOf(object).Kind() != reflect.Map {
		j, _ := json.Marshal(object)
		if bytes.Equal([]byte("true"), j) || bytes.Equal([]byte("false"), j) {
			// because Python's boolean is capitalized
			return bytes.Title(j)
		}
		return j
	}

	// order maps for reproducability
	m := object.(map[string]interface{})
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	b.WriteString("struct(\n")
	for _, key := range keys {
		b.Write(indent2)
		b.WriteString(key + "=")
		b.Write(a.genStarlarkStructWithIndent(m[key], indent+1))
		b.WriteString(",\n")
	}
	b.Write(indent1)
	b.WriteString(")")
	return b.Bytes()
}
