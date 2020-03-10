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
	"encoding/json"
	"fmt"
	"path"
	"sync"

	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/errors"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
)

var _ State = &stateCheckpoint{}

type stateCheckpoint struct {
	mux               sync.RWMutex
	policyName        string
	cache             State
	checkpointManager checkpointmanager.CheckpointManager
	checkpointName    string
}

// NewCheckpointState creates new State for keeping track of memory/pod assignment with checkpoint backend
func NewCheckpointState(stateDir, checkpointName, policyName string) (State, error) {
	var err error
	checkpointManager, err := checkpointmanager.NewCheckpointManager(stateDir)
	if err != nil {
		return nil, fmt.Errorf("[memorymanager] failed to initialize checkpoint manager: %v", err)
	}
	stateCheckpoint := &stateCheckpoint{
		cache:             NewMemoryState(),
		policyName:        policyName,
		checkpointManager: checkpointManager,
		checkpointName:    checkpointName,
	}

	if err := stateCheckpoint.restoreState(); err != nil {
		return nil, fmt.Errorf("[memorymanager] could not restore state from checkpoint: %v\n"+
			"Please drain this node and delete the Memory manager checkpoint file %q before restarting Kubelet.",
			err, path.Join(stateDir, checkpointName))
	}

	return stateCheckpoint, nil
}

func (sc *stateCheckpoint) restoreState() error {
	sc.mux.Lock()
	defer sc.mux.Unlock()
	var err error

	checkpoint := NewMemoryManagerCheckpoint()

	if err = sc.checkpointManager.GetCheckpoint(sc.checkpointName, checkpoint); err != nil {
		if err == errors.ErrCheckpointNotFound {
			sc.storeState()
			return nil
		}
		return err
	}

	if sc.policyName != checkpoint.PolicyName {
		return fmt.Errorf("[memorymanager] configured policy %q differs from state checkpoint policy %q", sc.policyName, checkpoint.PolicyName)
	}

	tmpAssignment := ContainerMemoryAssignments{}
	tmpMachine := topology.MemoryTopology{}

	containerBytes := []byte(checkpoint.ContainerAssignments)
	machineBytes := []byte(checkpoint.MachineMemory)

	if err = json.Unmarshal(containerBytes, &tmpAssignment); err != nil {
		return fmt.Errorf("[memorymanager] failed to unmarshal container memory assignment checkpoint")
	}

	if err = json.Unmarshal(machineBytes, &tmpMachine); err != nil {
		return fmt.Errorf("[memorymanager] failed to unmarshal machine memory checkpoint")
	}

	sc.cache.SetDefaultMemoryAssignments(tmpAssignment)
	sc.cache.SetMachineMemory(tmpMachine)

	klog.V(2).Info("[memorymanager] state checkpoint: restored state from checkpoint")

	return nil
}

// saves state to a checkpoint, caller is responsible for locking
func (sc *stateCheckpoint) storeState() {
	checkpoint := NewMemoryManagerCheckpoint()
	checkpoint.PolicyName = sc.policyName

	defaultAssignment := sc.cache.GetDefaultMemoryAssignments()
	machineMemory := sc.cache.GetMachineMemory()

	containerJSON, err := json.Marshal(defaultAssignment)

	if err != nil {
		panic("[memorymanager] could not marshal container memory assignment checkpoint: " + err.Error())
	}

	machineJSON, err := json.Marshal(machineMemory)

	if err != nil {
		panic("[memorymanager] could not marshal machine memory checkpoint: " + err.Error())
	}

	checkpoint.ContainerAssignments = string(containerJSON)
	checkpoint.MachineMemory = string(machineJSON)

	err = sc.checkpointManager.CreateCheckpoint(sc.checkpointName, checkpoint)

	if err != nil {
		panic("[memorymanager] could not save checkpoint: " + err.Error())
	}
}

func (sc *stateCheckpoint) GetMemoryAssignments(containerID string) (topology.MemoryTopology, bool) {
	sc.mux.RLock()
	defer sc.mux.RUnlock()

	return sc.cache.GetMemoryAssignments(containerID)
}

func (sc *stateCheckpoint) GetMachineMemory() topology.MemoryTopology {
	sc.mux.RLock()
	defer sc.mux.RUnlock()

	return sc.cache.GetMachineMemory()
}

func (sc *stateCheckpoint) GetDefaultMemoryAssignments() ContainerMemoryAssignments {
	sc.mux.RLock()
	defer sc.mux.RUnlock()

	return sc.cache.GetDefaultMemoryAssignments()
}

func (sc *stateCheckpoint) SetMemoryAssignments(containerID string, t topology.MemoryTopology) {
	sc.mux.Lock()
	defer sc.mux.Unlock()

	sc.cache.SetMemoryAssignments(containerID, t)
	sc.storeState()
}

func (sc *stateCheckpoint) SetMachineMemory(machineMemory topology.MemoryTopology) {
	sc.mux.Lock()
	defer sc.mux.Unlock()

	sc.cache.SetMachineMemory(machineMemory)
	sc.storeState()
}

func (sc *stateCheckpoint) SetDefaultMemoryAssignments(as ContainerMemoryAssignments) {
	sc.mux.Lock()
	defer sc.mux.Unlock()

	sc.cache.SetDefaultMemoryAssignments(as)
	sc.storeState()
}

func (sc *stateCheckpoint) Delete(containerID string) {
	sc.mux.Lock()
	defer sc.mux.Unlock()

	sc.cache.Delete(containerID)
	sc.storeState()
}

func (sc *stateCheckpoint) ClearState() {
	sc.mux.Lock()
	defer sc.mux.Unlock()

	sc.cache.ClearState()
	sc.storeState()
}
