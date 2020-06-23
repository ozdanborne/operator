package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tigera/operator/pkg/controller/migration/parser"
	appsv1 "k8s.io/api/apps/v1"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := appsv1.AddToScheme(scheme.Scheme); err != nil {
		return err
	}

	cl, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		return err
	}

	cfg, err := parser.GetExistingConfig(context.TODO(), cl)
	if err != nil {
		return err
	}

	bits, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	fmt.Println(string(bits))
	return nil
}
