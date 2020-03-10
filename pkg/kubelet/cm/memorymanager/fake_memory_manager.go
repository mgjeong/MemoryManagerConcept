/*
Copyright 2017 The Kubernetes Authors.

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

package memorymanager

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"

	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/status"
)

type fakeManager struct {
	state state.State
}

func (m *fakeManager) Start(activePods ActivePodsFunc, podStatusProvider status.PodStatusProvider, containerRuntime runtimeService) {
	klog.Info("[fake memorymanager] Start()")
}

func (m *fakeManager) Policy() Policy {
	klog.Info("[fake memorymanager] Policy()")
	return NewNonePolicy()
}

func (m *fakeManager) AddContainer(p *v1.Pod, c *v1.Container, containerID string, containerCgroupPath string) error {
	klog.Infof("[fake memorymanager] AddContainer (pod: %s, container: %s, container id: %s)", p.Name, c.Name, containerID)
	return nil
}

func (m *fakeManager) RemoveContainer(containerID string) error {
	klog.Infof("[fake memorymanager] RemoveContainer (container id: %s)", containerID)
	return nil
}

func (m *fakeManager) GetTopologyHints(pod v1.Pod, container v1.Container) map[string][]topologymanager.TopologyHint {
	klog.Infof("[fake memorymanager] Get Topology Hints")
	return map[string][]topologymanager.TopologyHint{}
}

func (m *fakeManager) State() state.Reader {
	return m.state
}

// NewFakeManager creates empty/fake memory manager
func NewFakeManager() Manager {
	return &fakeManager{
		state: state.NewMemoryState(),
	}
}
