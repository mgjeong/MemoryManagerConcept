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

package memorymanager

import (
	"fmt"
	"math"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/socketmask"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

const resourceHugePagesPrefix = "hugepages-"

// GetTopologyHints returns map which has key as name of resource.
func (m *manager) GetTopologyHints(pod v1.Pod, container v1.Container) map[string][]topologymanager.TopologyHint {
	var requested = GetRequestedMemory(&container)

	// Case of container doesn't needing memory, hugepages.
	if requested.MemSize == 0 && len(requested.HugePages) == 0 {
		return map[string][]topologymanager.TopologyHint{
			"memory": []topologymanager.TopologyHint{},
		}
	}

	containerID, _ := findContainerIDByName(&pod.Status, container.Name)
	memoryHints := getMemoryHintsAffinity(requested, containerID, m.state)
	calculatedHints := map[string][]topologymanager.TopologyHint{"memory": memoryHints}

	klog.Infof("[memorymanager] Calculated hints: %v", calculatedHints)

	return calculatedHints
}

// GetRequestedMemory returns MemoryNode which has requested memory or hugepages.
func GetRequestedMemory(container *v1.Container) topology.MemoryNode {
	var requested = topology.MemoryNode{
		HugePages: map[uint64]uint64{},
	}

	for resourceObj, amountObj := range container.Resources.Requests {
		resourceStr := string(resourceObj)

		if resourceStr != "memory" && !strings.HasPrefix(resourceStr, resourceHugePagesPrefix) {
			continue
		} else if resourceObj == "memory" {
			memoryAmount := amountObj.Value()
			// Casting int64 to uint64, because CRI interfaces use uint64, but Quantity use int64.
			requested.MemSize += uint64(memoryAmount)
		} else {
			hugepageAmountStr := strings.TrimPrefix(resourceStr, resourceHugePagesPrefix)
			hugepageSize, err := resource.ParseQuantity(hugepageAmountStr)
			if err != nil {
				klog.Infof("[memorymanager] fail to parse hugepage size")
				continue
			}
			// HugepageSize is divided by 1024 to ensure that the remainder is zero.
			hugepageSizeKB := uint64(hugepageSize.Value() / 1024)
			// HugepageAmount is the number of hugepages to use.
			hugepageAmount := uint64(math.Ceil(float64(amountObj.Value()) / float64(hugepageSize.Value())))
			requested.HugePages[hugepageSizeKB] += hugepageAmount
		}
	}

	return requested
}

// getMemoryHintsAffinity returns TopologyHint slice which can be deployed the container.
func getMemoryHintsAffinity(requested topology.MemoryNode, containerID string, state state.State) []topologymanager.TopologyHint {
	var memoryHints []topologymanager.TopologyHint

	memoryTopology := state.GetDefaultMemoryAssignments().Clone()

	// If exists memory topology with given container id, use them.
	if _, ok := memoryTopology[containerID]; ok {
		for nodeIdx := range memoryTopology[containerID] {
			mask, _ := socketmask.NewSocketMask(nodeIdx)
			memoryHints = append(memoryHints, topologymanager.TopologyHint{SocketAffinity: mask, Preferred: true})
		}
		return memoryHints
	}

	machineMemory := state.GetMachineMemory().Clone()
	currentMemoryUsageTopology := AccumulateMemory(memoryTopology)

	for nodeIdx := 0; nodeIdx < len(machineMemory); nodeIdx++ {
		nodeMemoryTopology := machineMemory[nodeIdx].Clone()
		nodeMemoryUsageTopology := currentMemoryUsageTopology[nodeIdx]

		nodeMemoryTopology.MemSize -= nodeMemoryUsageTopology.MemSize

		for hugepageSize, hugepageAmount := range nodeMemoryUsageTopology.HugePages {
			nodeMemoryTopology.HugePages[hugepageSize] -= hugepageAmount
		}

		if !checkAvailableMemory(nodeMemoryTopology, requested) {
			continue
		}
		// With current Memory Manager, all memory, hugepages are restricted in single NUMA node.
		mask, _ := socketmask.NewSocketMask(nodeIdx)
		memoryHints = append(memoryHints, topologymanager.TopologyHint{SocketAffinity: mask, Preferred: true})
	}

	// Prevent memory hints go to preferred any-socket hint by TopologyManager.
	if len(memoryHints) == 0 {
		memoryHints = append(memoryHints, topologymanager.TopologyHint{SocketAffinity: socketmask.NewEmptySocketMask(), Preferred: true})
	}

	return memoryHints
}

func checkAvailableMemory(available topology.MemoryNode, requested topology.MemoryNode) bool {
	if available.MemSize < requested.MemSize {
		return false
	}
	for size := range requested.HugePages {
		if available.HugePages[size] < requested.HugePages[size] {
			return false
		}
	}
	return true
}

func findContainerIDByName(status *v1.PodStatus, name string) (string, error) {
	allStatuses := status.InitContainerStatuses
	allStatuses = append(allStatuses, status.ContainerStatuses...)
	for _, container := range allStatuses {
		if container.Name == name && container.ContainerID != "" {
			cid := &kubecontainer.ContainerID{}
			err := cid.ParseString(container.ContainerID)
			if err != nil {
				return "", err
			}
			return cid.ID, nil
		}
	}
	return "", fmt.Errorf("unable to find ID for container with name %v in pod status (it may not be running)", name)
}
