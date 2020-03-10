package state

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
)

const testingCheckpoint = "memorymanager_checkpoint_test"

var testingDir = os.TempDir()

var normalTopology1 = &topology.MemoryTopology{
	0: {
		MemSize: 1073741824,
		HugePages: map[uint64]uint64{
			2048: 8,
		},
	},
	1: {
		MemSize: 1073741824,
		HugePages: map[uint64]uint64{
			1048576: 8,
		},
	},
}

var normalTopology2 = &topology.MemoryTopology{
	1: {
		MemSize: 1073741824,
		HugePages: map[uint64]uint64{
			2048: 8,
		},
	},
}

var normalMachineMemory = &topology.MemoryTopology{
	0: {
		MemSize: 15695687680,
		HugePages: map[uint64]uint64{
			1048576: 1,
			2048:    1,
		},
	},
	1: {
		MemSize: 15695687680,
		HugePages: map[uint64]uint64{
			1048576: 5,
		},
	},
}

var normalStateMemory = &stateMemory{
	memoryAssignments: ContainerMemoryAssignments{
		"container1": *normalTopology1,
		"container2": *normalTopology2,
	},
	machineMemory: *normalMachineMemory,
}

func TestCheckpointStateRestore(t *testing.T) {
	testCases := []struct {
		description          string
		containerAssignments string
		machineMemory        string
		policyName           string
		expectedState        *stateMemory
	}{
		{
			"Normal Case",
			`{"container1":{"0":{"memSize":1073741824,"hugePages":{"2048":8}},"1":{"memSize":1073741824,"hugePages":{"1048576":8}}},"container2":{"1":{"memSize":1073741824,"hugePages":{"2048":8}}}}`,
			`{"0":{"memSize":15695687680,"hugePages":{"1048576":1,"2048":1}},"1":{"memSize":15695687680,"hugePages":{"1048576":5}}}`,
			"none",
			normalStateMemory,
		},
	}

	// create checkpoint manager for testing
	cpm, err := checkpointmanager.NewCheckpointManager(testingDir)
	if err != nil {
		t.Fatalf("could not create testing checkpoint manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// ensure there is no previous checkpoint
			cpm.RemoveCheckpoint(testingCheckpoint)

			// prepare checkpoint for testing
			checkpoint := NewMemoryManagerCheckpoint()
			checkpoint.PolicyName = tc.policyName

			if strings.TrimSpace(tc.containerAssignments) != "" {
				checkpoint.ContainerAssignments = tc.containerAssignments
			}

			if strings.TrimSpace(tc.machineMemory) != "" {
				checkpoint.MachineMemory = tc.machineMemory
			}

			if err := cpm.CreateCheckpoint(testingCheckpoint, checkpoint); err != nil {
				t.Fatalf("could not create testing checkpoint: %v", err)
			}

			restoredState, err := NewCheckpointState(testingDir, testingCheckpoint, tc.policyName)
			t.Logf("%v", restoredState)

			if err != nil {
				t.Fatalf("unexpected error while creatng checkpointState: %v", err)
			}

			restoredState.GetMachineMemory()
			restoredState.GetDefaultMemoryAssignments()

			if !reflect.DeepEqual(restoredState.GetMachineMemory(), tc.expectedState.machineMemory) {
				t.Errorf("Difference in MachineMemory = %v, want %v", restoredState.GetMachineMemory(), tc.expectedState.machineMemory)
			}

			if !reflect.DeepEqual(restoredState.GetDefaultMemoryAssignments(), tc.expectedState.memoryAssignments) {
				t.Errorf("Difference in restored memory assignment = %v, want %v", restoredState.GetDefaultMemoryAssignments(), tc.expectedState.memoryAssignments)
			}
		})
	}
}

func TestCheckpointStateStore(t *testing.T) {
	testCases := []struct {
		description   string
		expectedState *stateMemory
	}{
		{
			"Store normal case",
			normalStateMemory,
		},
	}

	// create checkpoint manager for testing
	cpm, err := checkpointmanager.NewCheckpointManager(testingDir)
	if err != nil {
		t.Fatalf("could not create testing checkpoint manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// ensure there is no previous checkpoint
			cpm.RemoveCheckpoint(testingCheckpoint)

			cs1, err := NewCheckpointState(testingDir, testingCheckpoint, "none")
			if err != nil {
				t.Fatalf("could not create testing checkpointState instance: %v", err)
			}

			cs1.SetDefaultMemoryAssignments(tc.expectedState.memoryAssignments)

			// restore checkpoint with previously stored values
			cs2, err := NewCheckpointState(testingDir, testingCheckpoint, "none")
			if err != nil {
				t.Fatalf("could not create testing checkpointState instance: %v", err)
			}

			if !reflect.DeepEqual(cs1, cs2) {
				t.Errorf("Difference in checkpoint instance = %v, want %v", cs1, cs2)
			}
		})
	}
}
