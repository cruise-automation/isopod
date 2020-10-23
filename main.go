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

package main

import (
	"context"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"

	log "github.com/golang/glog"
	vaultapi "github.com/hashicorp/vault/api"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cruise-automation/isopod/pkg/cloud"
	"github.com/cruise-automation/isopod/pkg/dep"
	"github.com/cruise-automation/isopod/pkg/runtime"
	store "github.com/cruise-automation/isopod/pkg/store/kube"
	"github.com/cruise-automation/isopod/pkg/util"
)

var version = "<unknown>"

var (
	// optional
	vaultToken         = flag.String("vault_token", os.Getenv("VAULT_TOKEN"), "Vault token obtained during authentication.")
	namespace          = flag.String("namespace", "default", "Kubernetes namespace to store metadata in.")
	kubeconfig         = flag.String("kubeconfig", "", "Kubernetes client config path.")
	qps                = flag.Int("qps", 100, "qps to configure the kubernetes RESTClient")
	burst              = flag.Int("burst", 100, "the burst to configure the kubernetes RESTClient")
	addonRegex         = flag.String("match_addons", "", "Filters configured addons based on provided regex.")
	isopodCtx          = flag.String("context", "", "Comma-separated list of `foo=bar' context parameters passed to the clusters Starlark function.")
	dryRun             = flag.Bool("dry_run", false, "Print intended actions but don't mutate anything.")
	force              = flag.Bool("force", false, "Delete and recreate immutable resources without confirmation.")
	svcAcctKeyFile     = flag.String("sa_key", "", "Path to the service account json file.")
	noSpin             = flag.Bool("nospin", false, "Disables command line status spinner.")
	kubeDiff           = flag.Bool("kube_diff", false, "Print diff against live Kubernetes objects.")
	kubeDiffFilter     = util.StringsFlag("kube_diff_filter", []string{}, "Filter elements in diffs using JSONPath key matching.")
	kubeDiffFilterFile = flag.String("kube_diff_filter_file", "", "Path to a file of filters delimited by new lines.")
	showVersion        = flag.Bool("version", false, "Print binary version/system information and exit(0).")
	relativePath       = flag.String("rel_path", "", "The base path used to interpret double slash prefix.")
	depsFile           = flag.String("deps", "", "Path to isopod.deps")
)

func init() {
	stdlog.SetFlags(stdlog.Lshortfile)
}

func usageAndDie() {
	fmt.Fprintf(os.Stderr, `Isopod, an addons installer framework.

By default, isopod targets all addons on all clusters. One may confine the
selection with "--match_addons" and "--clusters_selector".

Usage: %s [options] <command> <ENTRYFILE_PATH | TEST_PATH | INPUT_PATH>

The following commands are supported:
	install        install addons
	remove         uninstall addons
	list           list addons in the ENTRYFILE_PATH
	test           run unit tests in TEST_PATH
	generate       generate a Starlark addon file from yaml or json file at INPUT_PATH

The following options are supported:
`, os.Args[0])
	flag.CommandLine.SetOutput(os.Stderr)
	flag.CommandLine.PrintDefaults()
	os.Exit(1)
}

func getCmdAndPath(argv []string) (cmd runtime.Command, path string) {
	if len(argv) < 1 {
		usageAndDie()
	}

	cmd = runtime.Command(argv[0])
	if len(argv) < 2 {
		if cmd == runtime.TestCommand {
			return
		}
		usageAndDie()
	}
	path = argv[1]
	return
}

func buildClustersRuntime(mainFile string) runtime.Runtime {
	clusters, err := runtime.New(&runtime.Config{
		EntryFile:         mainFile,
		GCPSvcAcctKeyFile: *svcAcctKeyFile,
		UserAgent:         "Isopod/" + version,
		KubeConfigPath:    *kubeconfig,
		DryRun:            *dryRun,
		Force:             *force,
	})
	if err != nil {
		log.Exitf("Failed to initialize clusters runtime: %v", err)
	}
	return clusters
}

func buildAddonsRuntime(kubeC *rest.Config, mainFile string) (runtime.Runtime, error) {
	vaultC, err := vaultapi.NewClient(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault client: %v", err)
	}
	if *vaultToken != "" {
		vaultC.SetToken(*vaultToken)
	}

	// configure rate limiter
	kubeC.QPS = float32(*qps)
	kubeC.Burst = *burst

	cs, err := kubernetes.NewForConfig(kubeC)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
	}
	helmBaseDir := *relativePath
	if helmBaseDir == "" {
		helmBaseDir = filepath.Dir(mainFile)
	}
	st := store.New(cs, *namespace)

	var diffFilters []string
	if *kubeDiffFilterFile != "" {
		diffFilters, err = util.LoadFilterFile(*kubeDiffFilterFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load diff filters: %v", err)
		}
	}
	if len(*kubeDiffFilter) > 0 {
		diffFilters = append(diffFilters, (*kubeDiffFilter)...)
	}

	opts := []runtime.Option{
		runtime.WithVault(vaultC),
		runtime.WithKube(kubeC, *kubeDiff, diffFilters),
		runtime.WithHelm(helmBaseDir),
		runtime.WithAddonRegex(regexp.MustCompile(*addonRegex)),
	}
	if *noSpin {
		opts = append(opts, runtime.WithNoSpin())
	}

	addons, err := runtime.New(&runtime.Config{
		EntryFile:         mainFile,
		GCPSvcAcctKeyFile: *svcAcctKeyFile,
		UserAgent:         "Isopod/" + version,
		KubeConfigPath:    *kubeconfig,
		Store:             st,
		DryRun:            *dryRun,
		Force:             *force,
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize addons runtime: %v", err)
	}

	return addons, nil
}

type verboseGlogWriter struct{}

func (w *verboseGlogWriter) Write(p []byte) (n int, err error) {
	log.V(1).Info(string(p))
	return len(p), nil
}

func main() {
	flag.Parse()
	ctx := context.Background()

	// Redirects all output to standrad Go log to Google's log at verbose level 1.
	stdlog.SetOutput(&verboseGlogWriter{})
	defer log.Flush()

	if *showVersion {
		fmt.Println("Version:", version)
		fmt.Printf("System: %s/%s\n", goruntime.GOOS, goruntime.GOARCH)
		return
	}

	cmd, path := getCmdAndPath(flag.Args())

	if *depsFile != "" {
		log.Infof("Loading dependencies from `%s'", *depsFile)
		if err := dep.Load(*depsFile); err != nil {
			log.Exitf("Failed to load deps file `%s': %v", *depsFile, err)
		}
	} else {
		// If depsFile unset, and if $(pwd)/isopod.deps exists, update depsFile.
		workingDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("Failed to get working dir: %v", err)
		}
		defaultDepsFilePath := filepath.Join(workingDir, dep.DepsFile)

		if _, err = os.Stat(defaultDepsFilePath); os.IsNotExist(err) {
			log.Info("Using no remote modules")
		}
		*depsFile = defaultDepsFilePath
	}

	if cmd == runtime.TestCommand {
		ok, err := runtime.RunUnitTests(ctx, path, os.Stdout, os.Stderr)
		if err != nil {
			log.Exitf("Failed to run tests: %v", err)
		} else if !ok {
			log.Flush()
			os.Exit(1)
		}
		return
	}

	if cmd == runtime.GenerateCommand {
		if err := runtime.Generate(path); err != nil {
			log.Exitf("Failed to generate Starlark code: %v", err)
		}
		return
	}

	mainFile := path
	if mainFile == "" {
		log.Exitf("path to main Starlark entry file must be set")
	}

	ctxParams, err := util.ParseCommaSeparatedParams(*isopodCtx)
	if err != nil {
		log.Exitf("Invalid value to --context: %v", err)
	}

	clusters := buildClustersRuntime(mainFile)
	if err := clusters.Load(ctx); err != nil {
		log.Exitf("Failed to load clusters runtime: %v", err)
	}

	errorReturned := false

	if err := clusters.ForEachCluster(ctx, ctxParams, func(k8sVendor cloud.KubernetesVendor) {
		kubeConfig, err := k8sVendor.KubeConfig(ctx)
		if err != nil {
			log.Exitf("Failed to build kube rest config for k8s vendor %v: %v", k8sVendor, err)
		}
		addons, err := buildAddonsRuntime(kubeConfig, mainFile)
		if err != nil {
			log.Exitf("Failed to initialize runtime: %v", err)
		}

		if err := addons.Load(ctx); err != nil {
			log.Exitf("Failed to load addons runtime: %v", err)
		}

		if err := addons.Run(ctx, cmd, k8sVendor.AddonSkyCtx(ctxParams)); err != nil {
			errorReturned = true
			log.Errorf("addons run failed: %v", err)
		}
	}); err != nil {
		log.Exitf("Failed to iterate through clusters: %v", err)
	}

	if errorReturned {
		os.Exit(2)
	}
}
