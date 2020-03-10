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
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/memorymanager/topology"
)

const (
	testPath = "./testdata/hugetlb"

	hugepage1Gi = "hugepages-1Gi"
	hugepage2Mi = "hugepages-2Mi"

	hugepage1GiFile = "hugetlb.1GB.limit_in_bytes"
	hugepage2MiFile = "hugetlb.2MB.limit_in_bytes"
)

func makePod(memoryRequest v1.ResourceList) *v1.Pod {
	containerStatus := v1.ContainerStatus{}
	containerStatus.Name = "container"
	containerStatus.ContainerID = "test://test"

	containerStatuses := []v1.ContainerStatus{containerStatus}

	return &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "container",
					Resources: v1.ResourceRequirements{
						Requests: memoryRequest,
						Limits:   memoryRequest,
					},
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: containerStatuses,
		},
	}
}

func Test_setHugetlbLimit(t *testing.T) {
	tests := []struct {
		name     string
		memory   topology.MemoryTopology
		path     string
		wantFile []string
		want     []uint64
		wantErr  bool
	}{
		{
			name: "Hugepage1Gi",
			memory: topology.MemoryTopology{
				0: {
					HugePages: map[uint64]uint64{
						1048576: 1,
					},
				},
			},
			path:     testPath,
			wantFile: []string{hugepage1GiFile},
			want:     []uint64{1048576 * 1024},
			wantErr:  false,
		},
		{
			name: "Hugepage2Mi",
			memory: topology.MemoryTopology{
				0: {
					HugePages: map[uint64]uint64{
						2048: 2,
					},
				},
			},
			path:     testPath,
			wantFile: []string{hugepage2MiFile},
			want:     []uint64{2048 * 2 * 1024},
			wantErr:  false,
		},
		{
			name: "Hugepage1Giand2Mi",
			memory: topology.MemoryTopology{
				0: {
					HugePages: map[uint64]uint64{
						1048576: 1,
						2048:    2,
					},
				},
			},
			path:     testPath,
			wantFile: []string{hugepage1GiFile, hugepage2MiFile},
			want:     []uint64{1048576 * 1024, 2048 * 2 * 1024},
			wantErr:  false,
		},
		{
			name: "PathErr",
			memory: topology.MemoryTopology{
				0: {
					HugePages: map[uint64]uint64{
						2048: 2,
					},
				},
			},
			path:     filepath.Join(testPath, "test"),
			wantFile: []string{hugepage1GiFile},
			want:     []uint64{2048 * 2 * 1024},
			wantErr:  true,
		},
	}

	if err := os.MkdirAll(testPath, 0755); err != nil {
		t.Errorf("Fail to make directory, setHugetlbLimit() test error = %v", err)
		return
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := setHugetlbLimit(test.memory, test.path)
			if err != nil {
				if test.wantErr {
					t.Logf("setHugetlbLimit() expected error = %v", err)
				} else {
					t.Errorf("setHugetlbLimit() error = %v, wantErr %v", err, test.wantErr)
				}
				return
			}

			for idx, file := range test.wantFile {
				path := filepath.Join(test.path, file)

				val, err := ioutil.ReadFile(path)
				if err != nil {
					t.Errorf("setHugetlbLimit() error to file open %v", err)
				}

				ret, err := strconv.ParseUint(string(bytes.TrimSpace(val)), 10, 64)
				if err != nil {
					t.Errorf("setHugetlbLimit() parse value to uint %v", err)
				}

				if !reflect.DeepEqual(ret, test.want[idx]) {
					t.Errorf("setHugetlbLimit() = %v, want %v", ret, test.want[idx])
				}
			}
		})
	}
}
