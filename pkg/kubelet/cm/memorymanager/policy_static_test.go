package memorymanager

import (
	"reflect"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
)

func Test_validateState(t *testing.T) {
	testCases := []struct {
		description         string
		machineTopology     *topology.MemoryTopology
		containerAssignment *state.ContainerMemoryAssignments
		expectedPanic       bool
	}{
		{
			description: "invalid node index in tmp",
			machineTopology: &topology.MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
				1: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
			},
			containerAssignment: &state.ContainerMemoryAssignments{
				"container1": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
				"container2": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					2: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
			},
			expectedPanic: true,
		},
		{
			description: "invalid hugepage size",
			machineTopology: &topology.MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
				1: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
			},
			containerAssignment: &state.ContainerMemoryAssignments{
				"container1": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
				"container2": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2049:    1,
						},
					},
				},
			},
			expectedPanic: true,
		},
		{
			description: "lack of memory",
			machineTopology: &topology.MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
				1: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
			},
			containerAssignment: &state.ContainerMemoryAssignments{
				"container1": {
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
							1048576: 1,
							2048:    1,
						},
					},
				},
				"container2": {
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
							1048576: 1,
							2048:    1,
						},
					},
				},
			},
			expectedPanic: true,
		},
		{
			description: "lack of hugepage",
			machineTopology: &topology.MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
				1: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
			},
			containerAssignment: &state.ContainerMemoryAssignments{
				"container1": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 2,
							2048:    2,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
				"container2": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
			},
			expectedPanic: true,
		},
		{
			description: "normal case",
			machineTopology: &topology.MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
				1: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    2,
					},
				},
			},
			containerAssignment: &state.ContainerMemoryAssignments{
				"container1": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
				"container2": {
					0: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
					1: {
						MemSize: 4194304,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
			},
			expectedPanic: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			defer func() {
				if err := recover(); err != nil {
					if !testCase.expectedPanic {
						t.Errorf("unexpected panic occurred: %q", err)
					}
				} else if testCase.expectedPanic {
					t.Error("expected panic doesn't occurred")
				}
			}()
			policy := NewStaticPolicy(testCase.machineTopology, topologymanager.NewFakeManager())

			s := state.NewMemoryState()
			s.SetDefaultMemoryAssignments(*testCase.containerAssignment)
			s.SetMachineMemory(*testCase.machineTopology)

			policy.Start(s)
		})
	}
}

func Test_AccumulateMemory(t *testing.T) {
	tests := []struct {
		description                string
		containerMemoryAssignments state.ContainerMemoryAssignments
		exectedTopology            topology.MemoryTopology
	}{
		{
			description: "normal case",
			containerMemoryAssignments: state.ContainerMemoryAssignments{
				"container1": {
					0: {
						MemSize: 123456,
						HugePages: map[uint64]uint64{
							1048576: 5,
							2048:    1,
						},
					},
					1: {
						MemSize: 123456,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    1,
						},
					},
				},
				"container2": {
					0: {
						MemSize: 999999999,
						HugePages: map[uint64]uint64{
							2048: 1,
						},
					},
					1: {
						MemSize: 0,
						HugePages: map[uint64]uint64{
							1048576: 1,
							2048:    100,
						},
					},
				},
			},
			exectedTopology: topology.MemoryTopology{
				0: {
					MemSize: 1000123455,
					HugePages: map[uint64]uint64{
						1048576: 5,
						2048:    2,
					},
				},
				1: {
					MemSize: 123456,
					HugePages: map[uint64]uint64{
						1048576: 2,
						2048:    101,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			accumulatedMemory := AccumulateMemory(test.containerMemoryAssignments)
			if !reflect.DeepEqual(accumulatedMemory, test.exectedTopology) {
				t.Errorf("AccumulateMemory() = %v, want %v", accumulatedMemory, test.exectedTopology)
			}
		})
	}

}
