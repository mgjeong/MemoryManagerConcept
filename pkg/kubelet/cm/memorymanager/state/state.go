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

import "k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"

// Reader interface used to read current memory assignment state
type Reader interface {
	GetMemoryAssignments(containerID string) (topology.MemoryTopology, bool)
	GetMachineMemory() topology.MemoryTopology
	GetDefaultMemoryAssignments() ContainerMemoryAssignments
}

type writer interface {
	SetMemoryAssignments(containerID string, t topology.MemoryTopology)
	SetMachineMemory(t topology.MemoryTopology)
	SetDefaultMemoryAssignments(as ContainerMemoryAssignments)
	Delete(containerID string)
	ClearState()
}

// State interface provides methods for tracking and setting memory assignment
type State interface {
	Reader
	writer
}
