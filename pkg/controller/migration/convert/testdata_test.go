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
	"fmt"
	"io"
	"os"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
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
	r, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	kdd := kyaml.NewDocumentDecoder(r)

	if err := apiextensions.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	codecs := serializer.NewCodecFactory(scheme.Scheme)
	rd := codecs.UniversalDeserializer()
	for {
		bits := make([]byte, 1024*100)
		i, err := kdd.Read(bits)
		if err != nil {
			if err == io.EOF {
				r.Close()
				break
			}
			return nil, fmt.Errorf("%v (%s)", err, string(bits))
		}

		rto, _, err := rd.Decode(bits[:i], nil, nil)
		if err != nil {
			// calico manifests commonly contain empty yaml documents, or start with a '---'. ignore these empty documents
			if runtime.IsMissingKind(err) {
				continue
			}
			return nil, err
		}
		rtos = append(rtos, rto)
	}

	return rtos, nil
}
