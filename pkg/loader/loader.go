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

package loader

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/golang/glog"
	"go.starlark.net/starlark"
)

// ModulesLoader defines the interface to interact with a ModulesLoader.
type ModulesLoader interface {
	// Load implements module loading. Repeated calls with the same module name
	// returns the same module.
	Load(t *starlark.Thread, module string) (starlark.StringDict, error)

	// GetLoaded returns a mapping of loaded module paths to their text context.
	GetLoaded() map[string]string
}

// Module represents a starlark modules.
type Module struct {
	globals starlark.StringDict
	data    []byte
	err     error
}

// ModulesLoader supports loading modules. In Starlark, each file is a module.
type modulesLoader struct {
	baseDir         string
	loaded          map[string]*Module
	predeclaredPkgs starlark.StringDict
}

// NewModulesLoader creates a new loader for modules.
func NewModulesLoader(baseDir string) ModulesLoader {
	return NewModulesLoaderWithPredeclaredPkgs(baseDir, nil)
}

// NewModulesLoaderWithPredeclaredPkgs creates a new loader for modules with
// predeclared packages.
func NewModulesLoaderWithPredeclaredPkgs(
	baseDir string,
	predeclaredPkgs starlark.StringDict,
) ModulesLoader {
	return &modulesLoader{
		baseDir:         baseDir,
		loaded:          map[string]*Module{},
		predeclaredPkgs: predeclaredPkgs,
	}
}

// Load implements module loading. Repeated calls with the same module name
// returns the same module.
func (l *modulesLoader) Load(_ *starlark.Thread, module string) (starlark.StringDict, error) {
	return l.anchoredLoadFn(l.baseDir, nil)(nil, module)
}

// anchoredLoadFn loads modules relative to the baseDir. It accepts a ModuleReaderFactory
// to allow unit testing with mocked readers.
func (l *modulesLoader) anchoredLoadFn(
	baseDir string,
	mockReaderFn *ModuleReaderFactory,
) func(t *starlark.Thread, module string) (starlark.StringDict, error) {
	return func(t *starlark.Thread, module string) (starlark.StringDict, error) {
		m, ok := l.loaded[module]
		if m != nil {
			return m.globals, m.err
		}
		if ok {
			return nil, errors.New("cycle in load graph")
		}

		// Add a placeholder to indicate "load in progress".
		l.loaded[module] = nil

		var predeclared starlark.StringDict
		switch ext := filepath.Ext(module); ext {
		case ".ipd", ".star":
			predeclared = l.predeclaredPkgs
		default:
			return nil, fmt.Errorf("unknown file extension: %s", ext)
		}
		readerFn := NewFileReaderFactory(baseDir)
		if mockReaderFn != nil {
			readerFn = *mockReaderFn
		}
		r, closer, err := readerFn(module)
		if err != nil {
			return nil, err
		}
		defer closer()
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}

		// Load and initialize the module in a new thread.
		newBaseDir := filepath.Join(baseDir, filepath.Dir(module))
		loadFn := l.anchoredLoadFn(newBaseDir, mockReaderFn)
		thread := &starlark.Thread{Load: loadFn}
		globals, err := starlark.ExecFile(thread, module, data, predeclared)
		m = &Module{globals: globals, data: data, err: err}

		// Update the cache.
		l.loaded[module] = m
		return m.globals, m.err
	}
}

func (l *modulesLoader) GetLoaded() map[string]string {
	modules := make(map[string]string, len(l.loaded))
	for m, v := range l.loaded {
		modules[m] = string(v.data)
	}
	return modules
}

// fakeModulesLoader implements ModulesLoader interface.
type fakeModulesLoader struct {
	modReaderFn ModuleReaderFactory
	*modulesLoader
}

// NewFakeModulesLoader creates a fake loader for modules with
// predeclared packages.
func NewFakeModulesLoader(
	predeclaredPkgs starlark.StringDict,
	modReaderFn ModuleReaderFactory,
) ModulesLoader {
	return &fakeModulesLoader{
		modReaderFn:   modReaderFn,
		modulesLoader: NewModulesLoaderWithPredeclaredPkgs("", predeclaredPkgs).(*modulesLoader),
	}
}

func (f *fakeModulesLoader) Load(_ *starlark.Thread, module string) (starlark.StringDict, error) {
	return f.anchoredLoadFn(f.baseDir, &f.modReaderFn)(nil, module)
}

// ModuleReaderFactory is a factory function returning reader for the module.
type ModuleReaderFactory func(module string) (r io.Reader, closeFn func(), err error)

// NewFileReaderFactory returns new ModuleReaderFactory function for reading
// from disk using path relative to baseDir. It will try to follow
// symlink to avoid module cycles.
// TODO(dmitry-ilyevskiy): Support git:// source.
func NewFileReaderFactory(baseDir string) ModuleReaderFactory {
	return func(module string) (io.Reader, func(), error) {
		mPath := filepath.Join(baseDir, module)
		module, err := filepath.EvalSymlinks(mPath)

		if err != nil {
			return nil, nil, err
		}

		f, err := os.Open(module)
		if err != nil {
			return nil, nil, err
		}

		closeFn := func() {
			if err := f.Close(); err != nil {
				log.Errorf("failed to close reader: %v", err)
			}
		}
		return f, closeFn, nil
	}
}
