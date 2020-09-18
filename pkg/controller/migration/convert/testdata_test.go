// Copyright (c) 2020 Tigera, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// this cli tool initializes and runs the conversion package which converts
// an existing manifest install of Calico into an installation object which represents it.
package convert

import (
	"bytes"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func calicoDefaultConfig() ([]runtime.Object, error) {
	return loadYaml("testdata/calico.yaml")
}

func awsCNIPolicyOnlyConfig() ([]runtime.Object, error) {
	return loadYaml("testdata/aws-cni-policy-only.yaml")
}

func loadYaml(filename string) ([]runtime.Object, error) {
	var rtos = []runtime.Object{}
	allBits, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	byteObjs := bytes.Split(allBits, []byte("---"))

	for _, bits := range byteObjs {
		rd := scheme.Codecs.UniversalDeserializer()
		rto, _, err := rd.Decode(bits, nil, nil)
		if err != nil {
			// calico manifests commonly contain empty yaml documents, or start with a '---'. ignore these empty documents
			continue
		}
		rtos = append(rtos, rto)
	}

	return rtos, nil
}
