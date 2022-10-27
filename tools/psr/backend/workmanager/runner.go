// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package workmanager

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/verrazzano/verrazzano/pkg/log/vzlog"
	"github.com/verrazzano/verrazzano/tools/psr/backend/config"
	"github.com/verrazzano/verrazzano/tools/psr/backend/metrics"
	"github.com/verrazzano/verrazzano/tools/psr/backend/spi"
	"sync/atomic"
	"time"
)

const (
	LoopCount     = "loop_count"
	LoopCountHelp = "The number of loop iterations executed"

	WorkerIterationNanoSeconds     = "worker_last_iteration_nanoseconds"
	WorkerIterationNanoSecondsHelp = "The total number of nanoseconds that the worker took to run the last iteration of doing work"

	WorkerDurationTotalSeconds     = "worker_total_seconds"
	WorkerDurationSecondsTotalHelp = "The total number of seconds that the worker has been running"
)

// WorkerRunner interface specifies a runner that loops calling a worker
type WorkerRunner interface {
	// RunWorker runs the worker use case in a loop
	RunWorker(config.CommonConfig, vzlog.VerrazzanoLogger) error

	// WorkerMetricsProvider is an interface to get prometheus metrics information for the worker to do work
	spi.WorkerMetricsProvider
}

// runner is needed to run the worker
type runner struct {
	spi.Worker
	metricDescList []prometheus.Desc
	*runnerMetrics
}

var _ WorkerRunner = runner{}

// runnerMetrics holds the metrics produced by the runner. Metrics must be thread safe.
type runnerMetrics struct {
	loopCount                  metrics.MetricItem
	workerIterationNanoSeconds metrics.MetricItem
	workerDurationTotalSeconds metrics.MetricItem
}

// NewRunner creates a new runner
func NewRunner(worker spi.Worker, conf config.CommonConfig, log vzlog.VerrazzanoLogger) (WorkerRunner, error) {
	constLabels := prometheus.Labels{}

	r := runner{Worker: worker, runnerMetrics: &runnerMetrics{}}

	d := prometheus.NewDesc(
		prometheus.BuildFQName(metrics.PsrNamespace, worker.GetWorkerDesc().MetricsName, LoopCount),
		LoopCountHelp,
		nil,
		constLabels,
	)
	r.metricDescList = append(r.metricDescList, *d)
	r.runnerMetrics.loopCount.Desc = d

	d = prometheus.NewDesc(
		prometheus.BuildFQName(metrics.PsrNamespace, worker.GetWorkerDesc().MetricsName, WorkerDurationTotalSeconds),
		WorkerDurationSecondsTotalHelp,
		nil,
		constLabels,
	)
	r.metricDescList = append(r.metricDescList, *d)
	r.runnerMetrics.workerDurationTotalSeconds.Desc = d

	d = prometheus.NewDesc(
		prometheus.BuildFQName(metrics.PsrNamespace, worker.GetWorkerDesc().MetricsName, WorkerIterationNanoSeconds),
		WorkerIterationNanoSecondsHelp,
		nil,
		constLabels,
	)
	r.metricDescList = append(r.metricDescList, *d)
	r.runnerMetrics.workerIterationNanoSeconds.Desc = d

	return r, nil
}

// GetMetricDescList returns the prometheus metrics descriptors for the worker metrics.  Must be thread safe
func (r runner) GetMetricDescList() []prometheus.Desc {
	return r.metricDescList
}

// GetMetricList returns the realtime metrics for the worker.  Must be thread safe
func (r runner) GetMetricList() []prometheus.Metric {
	metrics := []prometheus.Metric{}

	m := prometheus.MustNewConstMetric(
		r.runnerMetrics.loopCount.Desc,
		prometheus.GaugeValue,
		float64(atomic.LoadInt64(&r.runnerMetrics.loopCount.Val)))
	metrics = append(metrics, m)

	m = prometheus.MustNewConstMetric(
		r.runnerMetrics.workerDurationTotalSeconds.Desc,
		prometheus.GaugeValue,
		float64(atomic.LoadInt64(&r.runnerMetrics.workerDurationTotalSeconds.Val)))
	metrics = append(metrics, m)

	m = prometheus.MustNewConstMetric(
		r.runnerMetrics.workerIterationNanoSeconds.Desc,
		prometheus.GaugeValue,
		float64(atomic.LoadInt64(&r.runnerMetrics.workerIterationNanoSeconds.Val)))
	metrics = append(metrics, m)

	return metrics
}

// RunWorker runs the worker in a loop
func (r runner) RunWorker(conf config.CommonConfig, log vzlog.VerrazzanoLogger) error {
	startTimeSecs := time.Now().Unix()
	for {
		atomic.AddInt64(&r.runnerMetrics.loopCount.Val, 1)

		// call the wrapped worker.  Log any error but keep working
		startIteration := time.Now().UnixNano()
		err := r.Worker.DoWork(conf, log)
		if err != nil {
			log.Errorf("Failed calling %s to do work: %v", r.Worker.GetWorkerDesc().EnvName, err)
		}
		if r.Worker.WantIterationInfoLogged() {
			log.Infof("Loop Count: %v, Elapsed Secs: %v", r.runnerMetrics.loopCount, r.runnerMetrics.workerDurationTotalSeconds)
		}
		atomic.StoreInt64(&r.runnerMetrics.workerIterationNanoSeconds.Val, time.Now().UnixNano()-startIteration)
		atomic.StoreInt64(&r.runnerMetrics.workerDurationTotalSeconds.Val, time.Now().Unix()-startTimeSecs)

		time.Sleep(conf.IterationSleepNanos)
	}
}