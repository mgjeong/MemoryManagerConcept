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
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	v1qos "k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
)

// PolicyStatic is the name of the static policy
const PolicyStatic policyName = "static"

type staticPolicy struct {
	// memory socket topology
	topology *topology.MemoryTopology
	// topology manager reference to get container Topology affinity
	affinity topologymanager.Store
}

// Ensure staticPolicy implements Policy interface
var _ Policy = &staticPolicy{}

func NewStaticPolicy(topology *topology.MemoryTopology, affinity topologymanager.Store) Policy {
	return &staticPolicy{
		topology: topology,
		affinity: affinity,
	}
}

func (p staticPolicy) Name() string {
	return string(PolicyStatic)
}

func (p staticPolicy) Start(s state.State) {
	s.SetMachineMemory(p.topology.Clone())

	if err := p.validateState(s); err != nil {
		klog.Errorf("[memorymanager] static policy invalid state: %s\n", err.Error())
		panic("[memorymanager] - please drain node and remove policy state file")
	}
	klog.Info("[memorymanager] static policy: Start")
}

// AccumulateMemory merging memory information by node index.
func AccumulateMemory(c state.ContainerMemoryAssignments) topology.MemoryTopology {
	tmpMemoryTopology := topology.MemoryTopology{}

	for _, mt := range c {
		for idx, node := range mt {
			tempNode, exist := tmpMemoryTopology[idx]
			if !exist {
				tempNode = topology.MemoryNode{
					HugePages: map[uint64]uint64{},
					MemSize:   uint64(0),
				}
			}

			tempNode.MemSize += node.MemSize
			hp := node.HugePages
			for size, amount := range hp {
				tempNode.HugePages[size] += amount
			}
			tmpMemoryTopology[idx] = tempNode
		}
	}

	return tmpMemoryTopology
}

func (p staticPolicy) validateState(s state.State) error {
	tmpMemoryAssignment := s.GetDefaultMemoryAssignments()
	tmpMemoryTopology := AccumulateMemory(tmpMemoryAssignment)
	machineMemory := p.topology.Clone()

	// Check there is difference in machine state
	if !reflect.DeepEqual(machineMemory, s.GetMachineMemory()) {
		return fmt.Errorf("Invalid machine memory state")
	}

	// Validate that tmpMemoryTopology has correct node index, HugePages.
	for idx, node := range tmpMemoryTopology {
		memNode, err := machineMemory[idx]
		if !err {
			return fmt.Errorf("Invalid memory state")
		}

		hp := node.HugePages
		for size := range hp {
			_, err := memNode.HugePages[size]
			if !err {
				return fmt.Errorf("Invalid hugepage state")
			}
		}
	}

	// Make sure that machine memory has greater than or equal to sum of should be deployed pod's memory.
	for idx, node := range machineMemory {
		if _, exists := tmpMemoryTopology[idx]; !exists {
			continue
		}

		if machineMemory[idx].MemSize < tmpMemoryTopology[idx].MemSize {
			return fmt.Errorf("lack of memory")
		}
		tmpHugePage := tmpMemoryTopology[idx].HugePages
		machineHugePage := node.HugePages

		for size := range machineHugePage {
			if _, exists := machineHugePage[size]; exists {
				if machineHugePage[size] < tmpHugePage[size] {
					return fmt.Errorf("lack of hugepage")
				}
			}
		}
	}

	return nil
}

func (p *staticPolicy) AddContainer(s state.State, pod *v1.Pod, container *v1.Container, containerID string) error {
	if v1qos.GetPodQOS(pod) != v1.PodQOSGuaranteed {
		return nil
	} else if _, ok := s.GetMemoryAssignments(containerID); ok {
		klog.Infof("[memorymanager] static policy: container already present in state, skipping (container: %s, container id: %s)", container.Name, containerID)
		return nil
	}

	affinity := p.affinity.GetAffinity(string(pod.UID), container.Name).SocketAffinity

	sockets := affinity.GetSockets()

	if len(sockets) == 0 {
		return fmt.Errorf("failed to allocate memory")
	}

	nodeIdx := sockets[0]

	result := topology.MemoryTopology{
		nodeIdx: GetRequestedMemory(container),
	}

	s.SetMemoryAssignments(containerID, result)

	return nil
}

func (p *staticPolicy) RemoveContainer(s state.State, containerID string) error {
	klog.Infof("[memorymanager] static policy: RemoveContainer (container id: %s)", containerID)
	if _, ok := s.GetMemoryAssignments(containerID); ok {
		s.Delete(containerID)
	}
	return nil
}
