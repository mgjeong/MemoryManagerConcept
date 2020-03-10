/*
Copyright 2019 The Kubernetes Authors.

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

package state

import (
	"sync"

	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
)

type stateMemory struct {
	sync.RWMutex
	memoryAssignments ContainerMemoryAssignments
	machineMemory     topology.MemoryTopology
}

// ContainerMemoryAssignments has NUMA nodes' information from each container ID
type ContainerMemoryAssignments map[string]topology.MemoryTopology

// Clone returns a copy of ContainerMemoryAssignments
func (as ContainerMemoryAssignments) Clone() ContainerMemoryAssignments {
	ret := ContainerMemoryAssignments{}
	for key, val := range as {
		ret[key] = val.Clone()
	}
	return ret
}

var _ State = &stateMemory{}

// NewMemoryState creates new State for keeping track of memory assignment
func NewMemoryState() State {
	klog.Infof("[memorymanager] initializing new in-memory state store")
	return &stateMemory{
		memoryAssignments: ContainerMemoryAssignments{},
	}
}

func (s *stateMemory) GetMemoryAssignments(containerID string) (topology.MemoryTopology, bool) {
	s.RLock()
	defer s.RUnlock()

	res, ok := s.memoryAssignments[containerID]
	return res.Clone(), ok
}

func (s *stateMemory) GetMachineMemory() topology.MemoryTopology {
	s.RLock()
	defer s.RUnlock()

	return s.machineMemory
}

func (s *stateMemory) GetDefaultMemoryAssignments() ContainerMemoryAssignments {
	s.RLock()
	defer s.RUnlock()

	return s.memoryAssignments.Clone()
}

func (s *stateMemory) SetMemoryAssignments(containerID string, t topology.MemoryTopology) {
	s.Lock()
	defer s.Unlock()

	s.memoryAssignments[containerID] = t
}

func (s *stateMemory) SetMachineMemory(machineMemory topology.MemoryTopology) {
	s.Lock()
	defer s.Unlock()

	s.machineMemory = machineMemory
}

func (s *stateMemory) SetDefaultMemoryAssignments(memoryAssignments ContainerMemoryAssignments) {
	s.RLock()
	defer s.RUnlock()

	s.memoryAssignments = memoryAssignments
}

func (s *stateMemory) Delete(containerID string) {
	s.Lock()
	defer s.Unlock()

	delete(s.memoryAssignments, containerID)
}

func (s *stateMemory) ClearState() {
	s.Lock()
	defer s.Unlock()

	s.memoryAssignments = ContainerMemoryAssignments{}
}
