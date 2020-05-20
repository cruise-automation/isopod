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
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cruise-automation/isopod/pkg/kube"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const indentString = "    "

var out = func(format string, a ...interface{}) { fmt.Printf(format, a...) }

func Generate(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	yamlOrJSONFile, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	yamlsOrJSONs := bytes.Split(yamlOrJSONFile, []byte(`---`))
	a := newAddonFile()

	decode := serializer.NewCodecFactory(kube.Scheme).UniversalDeserializer().Decode

	for _, yamlOrJSON := range yamlsOrJSONs {
		if len(bytes.TrimSpace(yamlOrJSON)) == 0 {
			continue
		}
		obj, _, err := decode(yamlOrJSON, nil, nil)
		if err != nil {
			return err
		}
		a.addObject(obj)
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
	objects []k8sruntime.Object
	// names will be filled with the names of the objects. It's filled once genDataWithIndent() is called and the names
	// will be populated into this slice. Later it will be injected into kub_put, but outside the data array.
	// This separation is needed because all names will be discovered while uncovering object.Metadata during the data
	// generation using reflect, but the names are needed outside of that, on a higher level. So the names are stored
	// while they're discovered to avoid a second loop over object that drills down to name using reflect.
	names []string
}

func newAddonFile() *addonFile {
	return &addonFile{
		pkgMap: map[string]string{},
	}
}

func (a *addonFile) addObject(object k8sruntime.Object) {
	a.objects = append(a.objects, object)
}

func (a *addonFile) gen() []byte {
	buf := bytes.NewBuffer([]byte{})

	// vim tag for github and vim to render the file nicely
	buf.WriteString("# vim: set syntax=python:\n\n")

	// imports
	kubePut := a.genKubePutWithIndent(1)
	buf.Write(a.genImports())

	// install
	buf.WriteString("\ndef install(ctx):\n")
	buf.Write(kubePut)

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
	return imports.Bytes()
}

func (a *addonFile) genKubePutWithIndent(indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	indent2 := bytes.Repeat([]byte(indentString), indent+1)
	kubePut := bytes.NewBuffer([]byte{})
	for i, object := range a.objects {
		data := a.genDataWithIndent(reflect.ValueOf(object), indent+2)

		kubePut.Write(indent1)
		kubePut.WriteString("kube.put(\n")

		// name
		kubePut.Write(indent2)
		kubePut.WriteString("name=")
		kubePut.WriteString(a.names[i])
		kubePut.WriteString(",\n")

		// api_group
		apiGroup := object.GetObjectKind().GroupVersionKind().Group
		if apiGroup != "" {
			kubePut.Write(indent2)
			kubePut.WriteString("api_group=\"" + apiGroup + "\",\n")
		}

		// data
		kubePut.Write(indent2)
		kubePut.WriteString("data=[")
		kubePut.Write(data)
		kubePut.WriteString("]\n")

		kubePut.Write(indent1)
		kubePut.WriteString(")\n")

		if i != len(a.objects)-1 {
			kubePut.WriteString("\n")
		}
	}
	return kubePut.Bytes()
}

func (a *addonFile) genKubeDeleteWithIndent(indent int) []byte {
	indent1 := bytes.Repeat([]byte(indentString), indent)
	kubeDelete := bytes.NewBuffer([]byte{})
	for _, object := range a.objects {
		v := reflect.ValueOf(object)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		typeMeta := v.FieldByName("TypeMeta")
		objectMeta := v.FieldByName("ObjectMeta")
		if typeMeta.IsZero() || objectMeta.IsZero() {
			continue
		}
		kind := typeMeta.FieldByName("Kind")
		name := objectMeta.FieldByName("Name")
		namespace := objectMeta.FieldByName("Namespace")
		apiGroup := object.GetObjectKind().GroupVersionKind().Group
		if kind.IsZero() || name.IsZero() {
			continue
		}
		kubeDelete.Write(indent1)
		kubeDelete.WriteString("kube.delete(")
		kubeDelete.WriteString(strings.ToLower(kind.String()))
		kubeDelete.WriteString("=\"")
		if !namespace.IsZero() {
			kubeDelete.WriteString(namespace.String())
			kubeDelete.WriteString("/")
		}
		kubeDelete.WriteString(name.String())
		kubeDelete.WriteString("\"")
		if apiGroup != "" {
			kubeDelete.WriteString(fmt.Sprintf(", api_group=\"%s\"", apiGroup))
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
	if v.Kind() != reflect.Struct {
		j, _ := json.Marshal(v.Interface())
		if bytes.Equal([]byte("true"), j) || bytes.Equal([]byte("false"), j) {
			// because Python's boolean is capitalized
			return bytes.Title(j)
		}
		return j
	}

	t := v.Type()
	name, pkgPath := t.Name(), t.PkgPath()
	alias := a.addPkg(pkgPath)
	b.WriteString(alias + "." + name + "(\n")

	for i := 0; i < v.NumField(); i++ {
		vf := v.Field(i)
		if !vf.IsZero() {
			jsonTag := t.Field(i).Tag.Get("json")
			// this is even a slice of len 1, if jsonTag is "". So accessing 0 index is safe
			jsonTag = strings.Split(jsonTag, ",")[0]

			// add name in order for use in getKubePut
			if t == reflect.TypeOf(v1.ObjectMeta{}) && jsonTag == "name" {
				a.names = append(a.names, string(a.genDataWithIndent(vf, 0)))
			}

			if jsonTag == "" || jsonTag == "apiGroup" {
				continue
			}
			// add actual object
			b.Write(indent1)
			b.WriteString(jsonTag)
			b.WriteString("=")

			if vf.Kind() == reflect.Slice {
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
