package parser

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RaemonSet struct {
	appsv1.DaemonSet

	checkedVars map[string]checkedFields
}

func (r *RaemonSet) uncheckedVars() []string {
	unchecked := []string{}

	for _, t := range r.Spec.Template.Spec.Containers {
		for _, v := range t.Env {

			if _, ok := r.checkedVars[t.Name].envVars[v.Name]; !ok {
				unchecked = append(unchecked, t.Name+"/"+v.Name)
			}
		}
	}
	return unchecked
}

// getEnv gets the value of an environment variable and marks that it has been checked.
func (r *RaemonSet) getEnv(ctx context.Context, client client.Client, container string, key string) (*string, error) {
	c := getContainers(r.Spec.Template.Spec, container)
	if c == nil {
		return nil, ErrIncompatibleCluster{fmt.Sprintf("couldn't find %s container in existing calico-node daemonset", container)}
	}
	r.ignoreEnv(container, key)
	return getEnv(ctx, client, c.Env, key)
}

func (r *RaemonSet) ignoreEnv(container, key string) {
	if _, ok := r.checkedVars[container]; !ok {
		r.checkedVars[container] = checkedFields{
			map[string]bool{},
		}
	}
	r.checkedVars[container].envVars[key] = true
}
