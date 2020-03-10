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
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"

	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/status"

	units "github.com/docker/go-units"
	cgroupfs "github.com/opencontainers/runc/libcontainer/cgroups/fs"
	libcontainerconfigs "github.com/opencontainers/runc/libcontainer/configs"
)

// ActivePodsFunc is a function that returns a list of pods to reconcile.
type ActivePodsFunc func() []*v1.Pod

type runtimeService interface {
	UpdateContainerResources(id string, resources *runtimeapi.LinuxContainerResources) error
}

type policyName string

const memoryManagerStateFileName = "memory_manager_state"

// Manager interface provides methods for Kubelet to manage pod memory and hugepages
type Manager interface {
	// Start is called during Kubelet initialization.
	Start(activePods ActivePodsFunc, podStatusProvider status.PodStatusProvider, containerRuntime runtimeService)

	// AddContainer is called between container create and container start
	AddContainer(p *v1.Pod, c *v1.Container, containerID string, podCgroupPath string) error

	// RemoveContainer is called after Kubelet decides to kill or delete a
	// container.
	RemoveContainer(containerID string) error

	// State returns a read-only interface to the internal memory manager state.
	State() state.Reader

	// GetTopologyHints implements the Topology Manager Interface and is
	// consulted to make Topology aware resource alignments and returns
	// map which has key/value as resource name, topology hints.
	GetTopologyHints(pod v1.Pod, container v1.Container) map[string][]topologymanager.TopologyHint
}

type manager struct {
	sync.Mutex
	policy Policy

	// reconcilePeriod is the duration between calls to reconcileState.
	reconcilePeriod time.Duration

	// state allows pluggable memory assignment policies while sharing a common
	// representation of state for the system to inspect and reconcile.
	state state.State

	// containerRuntime is the container runtime service interface needed
	// to make UpdateContainerResources() calls against the containers.
	containerRuntime runtimeService

	// activePods is a method for listing active pods on the node
	// so all the containers can be updated in the reconciliation loop.
	activePods ActivePodsFunc

	// podStatusProvider provides a method for obtaining pod statuses
	// and the containerID of their containers
	podStatusProvider status.PodStatusProvider

	machineInfo *cadvisorapi.MachineInfo

	nodeAllocatableReservation v1.ResourceList
}

var _ Manager = &manager{}

// NewManager creates new memory manager based on provided policy
func NewManager(memoryPolicyName string, reconcilePeriod time.Duration, machineInfo *cadvisorapi.MachineInfo, nodeAllocatableAbsolute v1.ResourceList, stateFileDirectory string, affinity topologymanager.Store) (Manager, error) {
	var policy Policy

	switch policyName(memoryPolicyName) {
	case PolicyNone:
		policy = NewNonePolicy()

	case PolicyStatic:
		topo, err := topology.Discover(machineInfo)
		if err != nil {
			fmt.Errorf("[memorymanager] could not detect Memory topology: %v", err)
			return nil, err
		}
		klog.Infof("[memorymanager] detected Memory topology: %v", topo)
		policy = NewStaticPolicy(topo, affinity)

	default:
		fmt.Errorf("[memorymanager] Unknown policy \"%s\", falling back to default policy \"%s\"", memoryPolicyName, PolicyNone)
		policy = NewNonePolicy()
	}

	stateImpl, err := state.NewCheckpointState(stateFileDirectory, memoryManagerStateFileName, policy.Name())
	if err != nil {
		fmt.Errorf("could not initialize checkpoint manager: %v", err)
		return nil, err
	}

	manager := &manager{
		policy:                     policy,
		reconcilePeriod:            reconcilePeriod,
		state:                      stateImpl,
		machineInfo:                machineInfo,
		nodeAllocatableReservation: nodeAllocatableAbsolute,
	}
	return manager, nil
}

func (m *manager) Start(activePods ActivePodsFunc, podStatusProvider status.PodStatusProvider, containerRuntime runtimeService) {
	klog.Infof("[memorymanager] starting with %s policy", m.policy.Name())
	klog.Infof("[memorymanager] reconciling every %v", m.reconcilePeriod)

	m.activePods = activePods
	m.podStatusProvider = podStatusProvider
	m.containerRuntime = containerRuntime

	m.policy.Start(m.state)
	if m.policy.Name() == string(PolicyNone) {
		return
	}
	go wait.Until(func() { m.reconcileState() }, m.reconcilePeriod, wait.NeverStop)
}

func (m *manager) AddContainer(p *v1.Pod, c *v1.Container, containerID string, podCgroupPath string) error {
	m.Lock()
	err := m.policy.AddContainer(m.state, p, c, containerID)
	if err != nil {
		klog.Errorf("[memorymanager] AddContainer error: %v", err)
		m.Unlock()
		return err
	}

	memoryTopology, ok := m.state.GetMemoryAssignments(containerID)
	m.Unlock()

	// make sure that memoryTopology is valid object
	if ok == true && len(memoryTopology) != 0 {
		err = m.updateContainerCgroups(containerID, memoryTopology, podCgroupPath)
		if err != nil {
			klog.Errorf("[memorymanager] AddContainer error: %v", err)
			m.Lock()
			error := m.policy.RemoveContainer(m.state, containerID)
			if error != nil {
				klog.Errorf("[memorymanager] AddContainer rollback state error: %v", err)
			}
			m.Unlock()
		}

		klog.Infof("[memorymanager] AddContainer %s with topology: %v", containerID, memoryTopology)
		return err
	}

	klog.V(5).Infof("[memorymanager] update container resources is skipped due to memory topology is empty")
	return nil
}

func setHugetlbLimit(memoryTopology topology.MemoryTopology, containerCgroupPath string) error {
	var hugePageSizeList = []string{"kB", "MB", "GB", "TB", "PB"}

	subsystem := &cgroupfs.HugetlbGroup{}
	resources := &libcontainerconfigs.Resources{}
	hugepages := map[uint64]uint64{}

	for _, memoryNode := range memoryTopology {
		for size, amount := range memoryNode.HugePages {
			hugepages[size] += amount
		}
	}

	for size, amount := range hugepages {
		sizeStr := units.CustomSize("%g%s", float64(size), 1024.0, hugePageSizeList)

		resources.HugetlbLimit = append(resources.HugetlbLimit, &libcontainerconfigs.HugepageLimit{
			Pagesize: sizeStr,
			Limit:    amount * size * 1024, // Convert to byte
		})
	}

	libcontainerCgroupConfig := &libcontainerconfigs.Cgroup{
		Resources: resources,
	}

	if err := subsystem.Set(containerCgroupPath, libcontainerCgroupConfig); err != nil {
		return fmt.Errorf("[memorymanager] Failed to set config for supported subsystems error: %v", err)
	}

	return nil
}

func (m *manager) RemoveContainer(containerID string) error {
	m.Lock()
	defer m.Unlock()

	err := m.policy.RemoveContainer(m.state, containerID)
	if err != nil {
		klog.Errorf("[memorymanager] RemoveContainer error: %v", err)
		return err
	}
	return nil
}

func (m *manager) State() state.Reader {
	return m.state
}

type reconciledContainer struct {
	podName       string
	containerName string
	containerID   string
}

func (m *manager) reconcileState() (success []reconciledContainer, failure []reconciledContainer) {
	success = []reconciledContainer{}
	failure = []reconciledContainer{}
	return success, failure
}

func (m *manager) updateContainerCgroups(containerID string, memoryTopology topology.MemoryTopology, podCgroupPath string) error {
	cgroupType := "hugetlb"
	containerCgroupPath, err := getContainerCgroupPath(cgroupType, podCgroupPath, containerID)
	if err != nil {
		klog.Errorf("Failed to get container cgroup path %s", containerID)
	}

	if err := m.updateContainerCPUSet(containerID, memoryTopology); err != nil {
		return fmt.Errorf("Failed to update container cpuset.mems cgroup: %v", err)
	}

	if err := os.MkdirAll(containerCgroupPath, 0755); err != nil {
		return err
	}

	if err := setHugetlbLimit(memoryTopology, containerCgroupPath); err != nil {
		return fmt.Errorf("Failed to update container hugetlb cgroup: %v", err)
	}

	return nil
}

// getContainerCgroupParent return cotainer cgroup v1 path.
// ex) /sys/fs/cgroup/hugetlb/kubepods.slice/kubepods-<pod-id>.slice/docker-<containerid>.scope
// borrowed from vishvananda/netns/netns_linux.go
func getContainerCgroupPath(cgroupType string, podCgroupPath string, containerID string) (string, error) {
	// ToDo: need to fix this function after rebase k8s version 1.16(sewon.oh)
	cgroupRoot, err := findCgroupMountpoint(cgroupType)
	if err != nil {
		return "", err
	}
	return filepath.Join(cgroupRoot, podCgroupPath, "docker-"+containerID+".scope"), nil
	// attempts := []string{
	// 	filepath.Join(cgroupRoot, podCgroupPath, containerID),
	// 	// With more recent lxc versions use, cgroup will be in lxc/
	// 	filepath.Join(cgroupRoot, podCgroupPath, "lxc", containerID),
	// 	// With more recent docker, cgroup will be in docker/
	// 	filepath.Join(cgroupRoot, podCgroupPath, "docker", containerID),
	// 	// Even more recent docker versions under systemd use docker-<id>.scope/
	// 	filepath.Join(cgroupRoot, podCgroupPath, "docker-"+containerID+".scope"),
	// }
	// klog.Errorf("Failed to get container cgroup path %v", attempts)
	// var filename string
	// for _, attempt := range attempts {
	// 	matches, _ := filepath.Glob(attempt)
	// 	if len(matches) > 1 {
	// 		return "", fmt.Errorf("Ambiguous containerID supplied: %v", matches)
	// 	} else if len(matches) == 1 {
	// 		filename = matches[0]
	// 		break
	// 	}
	// }

	// if filename == "" {
	// 	return "", fmt.Errorf("Can not find container cgroup path %s", containerID)
	// }

	// return filename, nil
}

// borrowed from docker/utils/utils.go
func findCgroupMountpoint(cgroupType string) (string, error) {
	output, err := ioutil.ReadFile("/proc/mounts")
	if err != nil {
		return "", err
	}

	// /proc/mounts has 6 fields per line, one mount per line, e.g.
	// cgroup /sys/fs/cgroup/devices cgroup rw,relatime,devices 0 0
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, " ")
		if len(parts) == 6 && parts[2] == "cgroup" {
			for _, opt := range strings.Split(parts[3], ",") {
				if opt == cgroupType {
					return parts[1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("cgroup mountpoint not found for %s", cgroupType)
}

func (m *manager) updateContainerCPUSet(containerID string, memoryTopology topology.MemoryTopology) error {
	memoryNode := ""
	if memoryNode = nodeIndexStringBuilder(memoryTopology); memoryNode == "" {
		return nil
	}

	return m.containerRuntime.UpdateContainerResources(
		containerID,
		&runtimeapi.LinuxContainerResources{
			CpusetMems: memoryNode,
		})
}

func nodeIndexStringBuilder(memoryTopology topology.MemoryTopology) string {
	memoryNode := ""
	for socket := range memoryTopology {
		if memoryNode == "" {
			memoryNode = strconv.Itoa(socket)
			continue
		}
		memoryNode += "," + strconv.Itoa(socket)
	}
	return memoryNode
}
