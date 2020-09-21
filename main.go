/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	psdcloudv1 "github.com/nic-6443/oomkill-protector/api/v1"
	"github.com/nic-6443/oomkill-protector/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme        = runtime.NewScheme()
	setupLog      = ctrl.Log.WithName("setup")
	componentName = "OOMKillProtector"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = psdcloudv1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	currentNodeName := os.Getenv("KUBERNETES_NODE_NAME")
	protectorController := &controllers.OOMKillProtectorReconciler{
		Client:        mgr.GetClient(),
		EventRecorder: mgr.GetEventRecorderFor(componentName),
		Log:           ctrl.Log.WithName("controllers").WithName(componentName),
		Scheme:        mgr.GetScheme(),
		SelectorCache: map[string]map[string]labels.Selector{},
		NodeName:      currentNodeName,
	}
	if err = protectorController.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", componentName)
		os.Exit(1)
	}

	if err = (&controllers.PodReconciler{
		Client:              mgr.GetClient(),
		Log:                 ctrl.Log.WithName("controllers").WithName(componentName),
		Scheme:              mgr.GetScheme(),
		ProtectorController: protectorController,
		NodeName:            currentNodeName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", componentName)
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
