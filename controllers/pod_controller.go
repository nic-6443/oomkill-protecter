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

package controllers

import (
	"context"
	psdcloudv1 "github.com/nic-6443/oomkill-protector/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	ProtectorController *OOMKillProtectorReconciler
	NodeName            string
}

func (r *PodReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("pod", req.NamespacedName)
	done := ctrl.Result{}
	if _, ok := r.ProtectorController.SelectorCache[req.Namespace]; !ok {
		return done, nil
	}
	pod := &v1.Pod{}
	if err := r.Client.Get(context.TODO(), req.NamespacedName, pod); err != nil {
		return done, nil
	}
	for protectorName, selector := range r.ProtectorController.SelectorCache[req.Namespace] {
		if selector.Matches(labels.Set(pod.Labels)) {
			protector := &psdcloudv1.OOMKillProtector{}
			if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: req.Namespace, Name: protectorName}, protector); err != nil {
				continue
			}
			r.ProtectorController.CreateProtect(protector, pod)
			return done, nil
		}
	}
	return done, nil
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		Complete(r)
}
