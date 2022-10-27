// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package workmanager

import (
	"fmt"
	metrics2 "github.com/verrazzano/verrazzano/tools/psr/backend/metrics"
	"github.com/verrazzano/verrazzano/tools/psr/backend/workers/opensearch/logget"
	"os"

	"github.com/verrazzano/verrazzano/pkg/log/vzlog"
	"github.com/verrazzano/verrazzano/tools/psr/backend/config"
	"github.com/verrazzano/verrazzano/tools/psr/backend/spi"
	"github.com/verrazzano/verrazzano/tools/psr/backend/workers/example"
	"github.com/verrazzano/verrazzano/tools/psr/backend/workers/opensearch/loggen"
)

// RunWorker runs a worker to completion
func RunWorker(log vzlog.VerrazzanoLogger) error {
	// Get the common config for all the workers
	conf, err := config.GetCommonConfig(log)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// get the worker type
	wt := conf.WorkerType
	if len(wt) == 0 {
		log.Errorf("Failed, missing Env var PSR_WORKER_TYPE")
		os.Exit(1)
	}
	worker, err := getWorker(wt)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	// add the worker config
	if err := config.AddEnvConfig(worker.GetEnvDescList()); err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// init the runner and wrapped worker
	log.Infof("Initializing worker %s", wt)
	runner, err := NewRunner(worker, conf, log)
	if err != nil {
		log.Errorf("Failed initializing runner and worker: %v", err)
		os.Exit(1)
	}

	// start metrics server as go routine
	log.Info("Starting metrics server")
	mProviders := []spi.WorkerMetricsProvider{}
	mProviders = append(mProviders, runner)
	mProviders = append(mProviders, worker)
	go metrics2.StartMetricsServerOrDie(mProviders)

	// run the worker to completion (usually forever)
	log.Infof("Running worker %s", wt)
	err = runner.RunWorker(conf, log)
	return err
}

// getWorker returns a worker given the	 name of the worker
func getWorker(wt string) (spi.Worker, error) {
	switch wt {
	case config.WorkerTypeExample:
		return example.NewExampleWorker()
	case config.WorkerTypeLogGen:
		return loggen.NewLogGenerator()
	case config.WorkerTypeLogGet:
		return logget.NewLogGetter()
	default:
		return nil, fmt.Errorf("Failed, invalid worker type '%s'", wt)
	}
}