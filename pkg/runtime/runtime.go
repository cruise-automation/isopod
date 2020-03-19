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

package runtime

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	golog "log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/golang/glog"
	spin "github.com/tj/go-spin"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/addon"
	"github.com/cruise-automation/isopod/pkg/cloud"
	"github.com/cruise-automation/isopod/pkg/cloud/gke"
	"github.com/cruise-automation/isopod/pkg/cloud/onprem"
	"github.com/cruise-automation/isopod/pkg/loader"
	"github.com/cruise-automation/isopod/pkg/modules"
	"github.com/cruise-automation/isopod/pkg/store"
)

const (
	// InstallCommand will rollout configurations of all chosen addons by
	// calling the install(ctx) method in each addon.
	InstallCommand Command = "install"
	// RemoveCommand will uninstall all chosen addons by
	// calling the remove(ctx) method in each addon.
	RemoveCommand Command = "remove"
	// ListCommand will list all chosen addons in the Starlark entry file.
	ListCommand Command = "list"
	// TestCommand will run Isopod in unit test mode with external services
	// stubbed with mocks.
	TestCommand Command = "test"

	// ClustersStarFunc is the name of the function in Starlark that returns
	// a list of Starlark built-ins that implement cloud.KubernetesVendor
	// interface.
	ClustersStarFunc = "clusters"
	// AddonsStarFunc is the name of the function in Starlark that returns
	// a list of addon() built-ins.
	AddonsStarFunc = "addons"
)

// Command is the type of the supported Isopod runtime command.
type Command string

// Runtime describe the Isopod runtime behaviors.
type Runtime interface {
	// Load parses and resolves the main entry Starlark file.
	Load(ctx context.Context) error

	// Run starts the runtime and executes the given command in the main entry
	// Starlark file that has been loaded. The runtime will call AddonsStarFunc.
	Run(ctx context.Context, cmd Command, skyCtx starlark.Value) error

	// ForEachCluster calls the ClustersStarFunc in the main Starlark file with
	// userCtx as argument to get a list of Starlark built-ins that implement
	// the cloud.KubernetesVendor interface. It then iterates through each
	// cluster to call the user given fn.
	ForEachCluster(ctx context.Context, userCtx map[string]string, fn func(k8sVendor cloud.KubernetesVendor)) error
}

// runtime implements Runtime with Isopod builtins and globals from entry file.
type runtime struct {
	Config
	// filename string
	globals        starlark.StringDict
	pkgs           starlark.StringDict // Predeclared packages.
	addonRe        *regexp.Regexp
	store          store.Store
	noSpin, dryrun bool
}

func init() {
	// Enable Starlark features that are disabled by default.
	resolve.AllowFloat = true
	resolve.AllowSet = true
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowRecursion = true
}

// New returns new Runtime object from path with opts.
// TODO (cxu) refactor Config to Runtime and this param list to Config.
func New(c *Config, opts ...Option) (Runtime, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	options := &options{
		dryRun: c.DryRun,
		pkgs: starlark.StringDict{
			"error":  starlark.NewBuiltin("error", addon.ErrorFn),
			"sleep":  starlark.NewBuiltin("sleep", addon.SleepFn),
			"gke":    gke.NewGKEBuiltin(c.GCPSvcAcctKeyFile, c.UserAgent),
			"onprem": onprem.NewOnPremBuiltin(c.KubeConfigPath),
		},
	}
	for _, o := range opts {
		if err := o.apply(options); err != nil {
			return nil, fmt.Errorf("failed to apply options: %v", err)
		}
	}

	pkgs := options.pkgs
	pkgs["addon"] = addon.NewAddonBuiltin(filepath.Dir(c.EntryFile), options.pkgs)
	for n, pkg := range modules.Predeclared() {
		pkgs[n] = pkg
	}

	return &runtime{
		Config:  *c,
		pkgs:    pkgs,
		addonRe: options.addonRe,
		store:   c.Store,
		noSpin:  options.noSpin,
		dryrun:  options.dryRun,
	}, nil
}

func (r *runtime) Load(ctx context.Context) error {
	thread := &starlark.Thread{
		Print: printFn,
		Load:  loader.NewModulesLoaderWithPredeclaredPkgs(filepath.Dir(r.EntryFile), r.pkgs).Load,
	}

	data, err := ioutil.ReadFile(r.EntryFile)
	if err != nil {
		return err
	}

	r.globals, err = starlark.ExecFile(thread, r.EntryFile, data, r.pkgs)
	if err != nil {
		return err
	}
	return nil
}

// spinMsg prints spinner while waiting on errCh to return error, then exits.
func spinMsg(addonName string, errCh chan error) {
	s := spin.New()
	s.Set(spin.Spin1)
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			fmt.Printf("\r Installing %s... %s", addonName, s.Next())
		case err := <-errCh:
			if err != nil {
				fmt.Printf("\r Installing %s... err: %v\n", addonName, err)
			} else {
				fmt.Printf("\r Installing %s... done\n", addonName)
			}
			return
		}
	}
}

func (r *runtime) runCommand(ctx context.Context, cmd Command, addons []*addon.Addon) error {
	runUntilErr := func(addons []*addon.Addon, addonFn func(a *addon.Addon) error) error {
		for _, a := range addons {
			if err := addonFn(a); err != nil {
				return fmt.Errorf("%v run failed: %v", a, err)
			}
		}
		return nil
	}

	switch cmd {
	case ListCommand:
		var lstMsgs []string
		for _, a := range addons {
			lstMsgs = append(lstMsgs, a.StringPretty())
		}
		// TODO(dmitry-ilyevskiy): Print "live" status.
		fmt.Printf("Configured addons:\n\t%s\n", strings.Join(lstMsgs, "\n\t"))

	case InstallCommand:
		installAddonFn := func(a *addon.Addon) (err error) {
			pipeReader, pipeWriter := io.Pipe()
			golog.SetOutput(pipeWriter)
			buf := make([]byte, 1<<15)
			var bufStr string

			// Remove log spam from third-party libs.
			appendFilterBufStr := func(n int) {
				bufStr += string(buf[:n])
				lines := strings.Split(bufStr, "\n")
				if len(lines) == 1 {
					return
				}
				// Prints all lines but the last line.
				for i := 0; i < len(lines)-1; i++ {
					line := lines[i]
					if strings.Contains(line, "proto: ") || line == "" {
						continue
					}
					fmt.Println(line)
				}
				// Since the last line might be incomplete, keep it until
				// we see a newline.
				bufStr = lines[len(lines)-1]
			}
			go func() {
				for {
					n, err := pipeReader.Read(buf)
					if err != nil {
						if err == io.EOF {
							appendFilterBufStr(n)
							break
						}
						log.Errorf("error while reading from filtering pipe: %v", err)
						break
					}
					appendFilterBufStr(n)
					time.Sleep(100 * time.Millisecond)
				}
			}()

			defer func() {
				// Restore go log output and close pipe.
				golog.SetOutput(os.Stderr)
				pipeWriter.Close()
			}()

			if r.noSpin {
				return a.Install(ctx)
			}

			errCh := make(chan error)
			go spinMsg(a.Name, errCh)
			err = a.Install(ctx)
			errCh <- err
			return err
		}

		if r.dryrun {
			if err := runUntilErr(addons, installAddonFn); err != nil {
				return fmt.Errorf("failed addon installation: %v", err)
			}
			return nil
		}

		// Only create a rollout when not doing dryrun.
		rollout, err := r.store.CreateRollout()
		if err != nil {
			return fmt.Errorf("failed to initilize rollout state: %v", err)
		}

		fmt.Printf("Beginning rollout [%v] installation...\n", rollout.ID)

		if err := runUntilErr(addons, func(a *addon.Addon) (err error) {
			if err := installAddonFn(a); err != nil {
				return err
			}
			if _, err := r.store.PutAddonRun(rollout.ID, &store.AddonRun{
				Name:    a.Name,
				Modules: a.LoadedModules(),
				// TODO(dmitry-ilyevskiy): Fill in .Data and .ObjRefs.
			}); err != nil {
				return fmt.Errorf("failed to store run state for `%s' addon: %v", a.Name, err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed addon installation: %v", err)
		}

		if err := r.store.CompleteRollout(rollout.ID); err != nil {
			return fmt.Errorf("failed to commit `live' rollout state: %v", err)
		}

		fmt.Printf("Rollout [%v] is live!\n", rollout.ID)

	case RemoveCommand:
		return runUntilErr(addons, func(a *addon.Addon) error {
			return a.Remove(ctx)
		})
	default:
		return fmt.Errorf("command `%s' is not implemented", cmd)
	}
	return nil
}

func (r *runtime) Run(ctx context.Context, cmd Command, skyCtx starlark.Value) error {
	log.Infof("runtime running with `%v' command", cmd)

	ret, err := r.callStarlarkFunc(ctx, AddonsStarFunc, starlark.Tuple{skyCtx})
	if err != nil {
		return err
	}

	addonsList, ok := ret.(*starlark.List)
	if !ok {
		return fmt.Errorf("%v must be a list (got a %s)", ret, ret.Type())
	}

	var loaded []*addon.Addon
	var loadedNs []string
	for i := 0; i < addonsList.Len(); i++ {
		addonV := addonsList.Index(i)
		a, ok := addonV.(*addon.Addon)
		if !ok {
			return fmt.Errorf("%v is not an addon object (got a %s)", addonV, addonV.Type())
		}

		if r.addonRe != nil && !r.addonRe.MatchString(a.Name) {
			log.V(1).Infof("%v doesn't match filter regexp (%v), skipping...", a, r.addonRe)
			continue
		}

		if err := a.Load(ctx); err != nil {
			return fmt.Errorf("%v load failed: %v", a, err)
		}
		loaded = append(loaded, a)
		loadedNs = append(loadedNs, a.Name)
	}

	if len(loaded) == 0 {
		return fmt.Errorf("no addon matches the filter regexp")
	}

	log.Infof("Running `%s' for %v...", cmd, loadedNs)

	if err := r.runCommand(ctx, cmd, loaded); err != nil {
		return fmt.Errorf("`%v' execution failed: %v", cmd, err)
	}

	return err
}

func (r *runtime) callStarlarkFunc(ctx context.Context, fnName string, args starlark.Tuple) (starlark.Value, error) {
	entry, ok := r.globals[fnName]
	if !ok {
		return nil, fmt.Errorf("no %q function found in %q", fnName, r.EntryFile)
	}

	entryFn, ok := entry.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%s must be a function (got a %s)", entry, entry.Type())
	}

	log.V(1).Infof("Invoking %v with params (%v)", entryFn, args)
	thread := &starlark.Thread{
		Print: printFn,
	}
	thread.SetLocal("context", ctx)

	return starlark.Call(thread, entryFn, args, nil)
}

func goMapToSkyCtx(m map[string]string) *addon.SkyCtx {
	skyParams := make(starlark.StringDict, len(m))
	for k, v := range m {
		skyParams[k] = starlark.String(v)
	}
	return &addon.SkyCtx{Attrs: skyParams}
}

func (r *runtime) ForEachCluster(ctx context.Context, userCtx map[string]string, fn func(k8sVendor cloud.KubernetesVendor)) error {
	ret, err := r.callStarlarkFunc(ctx, "clusters", starlark.Tuple{goMapToSkyCtx(userCtx)})
	if err != nil {
		return fmt.Errorf("error when calling `clusters': %v ", err)
	}

	chosenClusters, ok := ret.(*starlark.List)
	if !ok {
		return fmt.Errorf("%v must be a list (got a `%s')", ret, ret.Type())
	}

	iter := chosenClusters.Iterate()
	defer iter.Done()
	var cluster starlark.Value
	for iter.Next(&cluster) {
		// Currently only supports GKE. Other vendors can easily be supported.
		k8sVendor, ok := cluster.(cloud.KubernetesVendor)
		if !ok {
			log.Errorf("Builtin `%v' does not implement cloud.KubernetesVendor interface. Skipping...", cluster)
			continue
		}

		clusterName := k8sVendor.AddonSkyCtx(userCtx).Attrs["cluster"]
		fmt.Printf("Current cluster: (%s)\n", clusterName)

		fn(k8sVendor)
	}
	return nil
}

func printFn(_ *starlark.Thread, msg string) { fmt.Println(msg) }
