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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/bytefmt"
	psdcloudv1 "github.com/nic-6443/oomkill-protector/api/v1"
	"github.com/nic-6443/oomkill-protector/protector/docker"
	"github.com/nic-6443/oomkill-protector/protector/memory"
)

const (
	OOMKillProtectorStart   = "OOMKillProtectorStart"
	DynamicProvisionSuccess = "DynamicProvisionSuccess"
	OriginalLimitHit        = "OriginalLimitHit"
)

type OOMKillProtectorReconciler struct {
	client.Client
	EventRecorder       record.EventRecorder
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	SelectorCache       map[string]map[string]labels.Selector
	NodeName            string
	thresholdHitEventCh chan *memory.MemoryProtect
	limitHitEventCh     chan *memory.MemoryProtect
}

func (r *OOMKillProtectorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("oomkillprotector", req.NamespacedName)

	done := ctrl.Result{}
	protector := &psdcloudv1.OOMKillProtector{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, protector)
	if err != nil {
		if _, ok := r.SelectorCache[req.Namespace][req.Name]; ok {
			delete(r.SelectorCache[req.Namespace], req.Name)
		}
		return done, nil
	}
	selector := labels.SelectorFromSet(protector.Spec.Selector)
	if _, ok := r.SelectorCache[req.Namespace]; !ok {
		r.SelectorCache[req.Namespace] = map[string]labels.Selector{}
	}
	r.SelectorCache[req.Namespace][req.Name] = selector
	podList := &v1.PodList{}
	if err := r.Client.List(context.TODO(), podList, &client.ListOptions{
		Namespace:     protector.Namespace,
		LabelSelector: selector,
	}); err != nil {
		r.Log.Error(err, "")
		return done, nil
	}
	if podList.Size() == 0 {
		return done, nil
	}

	for _, pod := range podList.Items {
		r.CreateProtect(protector, &pod)
	}
	return done, nil
}

func (r *OOMKillProtectorReconciler) CreateProtect(protector *psdcloudv1.OOMKillProtector, pod *v1.Pod) {
	thresholdRatio := float64(protector.Spec.ThresholdRatio) / 100
	scalaRatio := float64(protector.Spec.ScalaRatio) / 100
	if pod.Status.Phase != v1.PodRunning || pod.DeletionTimestamp != nil {
		return
	}
	if pod.Spec.NodeName != r.NodeName {
		return
	}
	pod.Spec.Containers[0].Resources.Limits.Memory().AsInt64()
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name != protector.Spec.ContainerName {
			continue
		}
		dockerClient, err := docker.GetClient("unix:///var/run/docker.sock")
		if err != nil {
			r.Log.Error(err, "get docker client error")
			r.EventRecorder.Event(protector, v1.EventTypeWarning, OOMKillProtectorStart, fmt.Sprintf("get docker client error"))
			continue
		}
		containerIDSlice := strings.SplitN(containerStatus.ContainerID, "://", 2)
		if len(containerIDSlice) != 2 {
			continue
		}
		container, err := dockerClient.GetContainerJSONById(containerIDSlice[1])
		if err != nil {
			r.Log.Error(err, "get container error")
			continue
		}
		r.Log.Info(fmt.Sprintf("requset protect for %v/%v/%v", pod.Namespace, pod.Name, container.Name))
		cgroupMemLimit, err := memory.GetMemoryLimit(container.State.Pid)
		if err != nil {
			continue
		}
		containerSpec := getContainer(pod, protector.Spec.ContainerName)
		if containerSpec == nil {
			continue
		}
		containerMemLimit, success := containerSpec.Resources.Limits.Memory().AsInt64()
		if !success {
			continue
		}
		if containerMemLimit != int64(cgroupMemLimit) {
			r.Log.Info(fmt.Sprintf("%v/%v/%v memory limit mismatch between kubernetes spec and cgroup, maybe already provisioned", pod.Namespace, pod.Name, container.Name))
			continue
		}
		err = memory.Protect(pod, container.State.Pid, thresholdRatio, scalaRatio, r.thresholdHitEventCh, r.limitHitEventCh)
		if err != nil {
			r.EventRecorder.Event(pod, v1.EventTypeWarning, OOMKillProtectorStart, fmt.Sprintf("create protect for %v/%v/%v failed", pod.Namespace, pod.Name, container.Name))
			continue
		}
		r.EventRecorder.Event(pod, v1.EventTypeNormal, OOMKillProtectorStart, fmt.Sprintf("start OOMKill protector for %v", containerStatus.Name))
	}
}

func getContainer(pod *v1.Pod, containerName string) *v1.Container {
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			return &container
		}
	}
	return nil
}

func (r *OOMKillProtectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.thresholdHitEventCh = make(chan *memory.MemoryProtect)
	r.limitHitEventCh = make(chan *memory.MemoryProtect)
	go func() {
		for memoryProtect := range r.thresholdHitEventCh {
			r.EventRecorder.Event((*memoryProtect).Pod, v1.EventTypeNormal, DynamicProvisionSuccess,
				fmt.Sprintf("Dynamic provision memory from %v to %v", bytefmt.ByteSize(uint64((*memoryProtect).OldMemLimit)), bytefmt.ByteSize(uint64((*memoryProtect).NewMemLimit))))
		}
	}()
	go func() {
		for memoryProtect := range r.limitHitEventCh {
			r.EventRecorder.Event((*memoryProtect).Pod, v1.EventTypeNormal, OriginalLimitHit, fmt.Sprintf("Memory hit original limit %v", bytefmt.ByteSize(uint64((*memoryProtect).OldMemLimit))))
		}
	}()
	return ctrl.NewControllerManagedBy(mgr).
		For(&psdcloudv1.OOMKillProtector{}).
		Complete(r)
}
