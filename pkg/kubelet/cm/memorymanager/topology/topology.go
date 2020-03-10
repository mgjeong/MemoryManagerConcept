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

package topology

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	"k8s.io/klog"
)

var (
	hugepageSizeRegexp = regexp.MustCompile(`hugepages-\s*([0-9]+)kB`)
)

const nodePath = "/sys/devices/system/node"

// MemoryTopology is a map from NUMA node index to MemoryNode.
type MemoryTopology map[int]MemoryNode

// MemoryNode represents memory capacity of single NUMA node.
// Total size of memory can be calculated by add MemSize and hugepages.
type MemoryNode struct {
	// Memory size of single NUMA node in byte,
	// mind that memSize does not contain persistent hugepages.
	// It can be calculated by "MemSize = MemTotal - (total memory of HugePages)"
	MemSize uint64 `json:"memSize"`

	// hugePages is a map from size of hugepage to number of hugepages.
	// for an example, map[2048(kB)]10 means a NUMA node has 10 of 2MB hugepage.
	HugePages map[uint64]uint64 `json:"hugePages"`
}

// Clone returns a copy of MemoryTopology
func (mt MemoryTopology) Clone() MemoryTopology {
	ret := MemoryTopology{}
	for key, val := range mt {
		ret[key] = val.Clone()
	}
	return ret
}

// Clone returns a copy of MemoryNode
func (mn MemoryNode) Clone() MemoryNode {
	ret := MemoryNode{
		MemSize:   mn.MemSize,
		HugePages: map[uint64]uint64{},
	}
	for key, val := range mn.HugePages {
		ret.HugePages[key] = val
	}
	return ret
}

// Discover collects memory and hugepages information from each NUMA nodes
// and returns reference of MemoryTopology
func Discover(machineInfo *cadvisorapi.MachineInfo, optionalPath ...string) (*MemoryTopology, error) {
	if machineInfo.MemoryCapacity == 0 {
		return nil, fmt.Errorf("could not detect memory capacity")
	}

	memoryToplogy := MemoryTopology{}
	var path string

	if len(optionalPath) == 0 {
		path = nodePath
	} else {
		path = optionalPath[0]
	}

	for _, socket := range machineInfo.Topology {
		hugePages, err := getNUMAHugepages(path, socket.Id)
		if err != nil {
			klog.Errorf("could not get hugepages for socket: %d", socket.Id)
			return nil, err
		}

		memSize := socket.Memory

		for pageSize, pageNum := range hugePages {
			//Size of hugepages * kB(1024) * Num of hugepages
			memSize -= pageSize * 1024 * pageNum
		}

		memoryToplogy[socket.Id] = MemoryNode{
			MemSize:   memSize,
			HugePages: hugePages,
		}
	}

	klog.Infof("[memorymanager] Discovered machine memory information: %v", memoryToplogy)
	return &memoryToplogy, nil
}

// borrowed from cadvisor/machine/machine.go
func getNUMAHugepages(nodePath string, nodeIndex int) (map[uint64]uint64, error) {
	hugePages := make(map[uint64]uint64)
	path := filepath.Join(nodePath, fmt.Sprintf("node%d/hugepages", nodeIndex))

	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		fileName := file.Name()

		pageSize, err := parseCapacity([]byte(fileName), hugepageSizeRegexp)
		if err != nil {
			return nil, err
		}

		file := filepath.Join(path, fileName, "free_hugepages")

		num, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}

		pageNum, err := strconv.ParseUint(string(bytes.TrimSpace(num)), 10, 64)
		if err != nil {
			return nil, err
		}

		hugePages[pageSize] = pageNum
	}

	return hugePages, nil
}

// borrowed from cadvisor/machine/machine.go
func parseCapacity(b []byte, r *regexp.Regexp) (uint64, error) {
	matches := r.FindSubmatch(b)
	if len(matches) != 2 {
		return 0, fmt.Errorf("failed to match regexp in output: %q", string(b))
	}

	m, err := strconv.ParseUint(string(matches[1]), 10, 64)
	if err != nil {
		return 0, err
	}

	return m, err
}
