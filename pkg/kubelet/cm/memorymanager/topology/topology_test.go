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
	"reflect"
	"testing"

	cadvisorapi "github.com/google/cadvisor/info/v1"
)

const testPath = "./testdata"

func Test_Discover(t *testing.T) {
	tests := []struct {
		name    string
		args    *cadvisorapi.MachineInfo
		want    *MemoryTopology
		wantErr bool
	}{
		{
			name: "FailMemoryCapacity",
			args: &cadvisorapi.MachineInfo{
				MemoryCapacity: 0,
			},
			want:    &MemoryTopology{},
			wantErr: true,
		},
		{
			name: "OneSocket",
			args: &cadvisorapi.MachineInfo{
				MemoryCapacity: 16771526656,
				Topology: []cadvisorapi.Node{
					{
						Id:     0,
						Memory: 16771526656,
					},
				},
			},
			want: &MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 1,
						2048:    1,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "DualSocket",
			args: &cadvisorapi.MachineInfo{
				MemoryCapacity: 16771526656,
				Topology: []cadvisorapi.Node{
					{
						Id:     0,
						Memory: 16771526656,
					},
					{
						Id:     1,
						Memory: 16771526656,
					},
				},
			},
			want: &MemoryTopology{
				0: {
					MemSize: 15695687680,
					HugePages: map[uint64]uint64{
						1048576: 1,
						2048:    1,
					},
				},
				1: {
					MemSize: 11402817536,
					HugePages: map[uint64]uint64{
						1048576: 5,
						2048:    0,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ret, err := Discover(test.args, testPath)
			if err != nil {
				if test.wantErr {
					t.Logf("Discover() expected error = %v", err)
				} else {
					t.Errorf("Discover() error = %v, wantErr %v", err, test.wantErr)
				}
				return
			}
			if !reflect.DeepEqual(ret, test.want) {
				t.Errorf("Discover() = %v, want %v", ret, test.want)
			}
		})
	}
}
