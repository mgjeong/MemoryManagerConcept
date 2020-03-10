package memorymanager

import (
	"reflect"
	"testing"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/state"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/socketmask"
)

func Test_GetRequestedMemory(t *testing.T) {
	tests := []struct {
		description      string
		memoryRequest    v1.ResourceList
		expectMemoryNode topology.MemoryNode
	}{
		{
			"normal case",
			v1.ResourceList{
				v1.ResourceName(v1.ResourceMemory): resource.MustParse("1G"),
				hugepage1Gi:                        resource.MustParse("2G"),
			},
			topology.MemoryNode{
				MemSize: 1000000000,
				HugePages: map[uint64]uint64{
					1048576: 2,
				},
			},
		},
		{
			"ceil",
			v1.ResourceList{
				hugepage1Gi: resource.MustParse("2500M"),
			},
			topology.MemoryNode{
				HugePages: map[uint64]uint64{
					1048576: 3,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			pod := makePod(test.memoryRequest)
			container := &pod.Spec.Containers[0]
			memoryNode := GetRequestedMemory(container)

			if !reflect.DeepEqual(memoryNode, test.expectMemoryNode) {
				t.Errorf("GetRequestedMemory() = %v, want %v", memoryNode, test.expectMemoryNode)
			}
		})
	}
}

func Test_GetTopologyHints(t *testing.T) {
	hugepages := []cadvisorapi.HugePagesInfo{
		cadvisorapi.HugePagesInfo{
			PageSize: 2048,
			NumPages: 16,
		},
		cadvisorapi.HugePagesInfo{
			PageSize: 1048576,
			NumPages: 16,
		},
	}

	m := manager{
		policy: &staticPolicy{},
		machineInfo: &cadvisorapi.MachineInfo{
			HugePages: hugepages,
			Topology: []cadvisorapi.Node{
				{
					Id:     0,
					Memory: 137438953472,
				},
				{
					Id:     1,
					Memory: 137438953472,
				},
			},
		},
		state: state.NewMemoryState(),
	}

	m.state.SetMachineMemory(topology.MemoryTopology{
		0: topology.MemoryNode{
			MemSize: 137438953472,
			HugePages: map[uint64]uint64{
				2048: 16,
			},
		},
		1: topology.MemoryNode{
			MemSize: 137438953472,
			HugePages: map[uint64]uint64{
				1048576: 16,
			},
		},
	})

	resource := v1.ResourceList{
		v1.ResourceName(v1.ResourceMemory): resource.MustParse("1G"),
		hugepage1Gi:                        resource.MustParse("2G"),
	}
	pod := makePod(resource)
	container := &pod.Spec.Containers[0]
	socketmask1, _ := socketmask.NewSocketMask(1)

	tests := []struct {
		description   string
		pod           v1.Pod
		container     v1.Container
		expectedHints map[string][]topologymanager.TopologyHint
	}{
		{
			"normal case",
			*pod,
			*container,
			map[string][]topologymanager.TopologyHint{
				"memory": []topologymanager.TopologyHint{
					topologymanager.TopologyHint{
						SocketAffinity: socketmask1,
						Preferred:      true,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			hints := m.GetTopologyHints(test.pod, test.container)
			if len(test.expectedHints) == 0 || len(hints) == 0 {
				return
			}
			if !reflect.DeepEqual(test.expectedHints, hints) {
				t.Errorf("Expected in result to be %v , got %v", hints, test.expectedHints)
			}
		})
	}
}
