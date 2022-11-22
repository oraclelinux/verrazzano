// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package mysql

import (
	"context"
	"fmt"
	"time"

	k8sready "github.com/verrazzano/verrazzano/pkg/k8s/ready"
	"github.com/verrazzano/verrazzano/pkg/log/vzlog"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/mysqloperator"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clipkg "sigs.k8s.io/controller-runtime/pkg/client"
)

// repairMySQLPodsWaitingReadinessGates - temporary workaround to repair issue were a MySQL pod
// can be stuck waiting for its readiness gates to be met.
func (c mysqlComponent) repairMySQLPodsWaitingReadinessGates(ctx spi.ComponentContext) error {
	podsWaiting, err := c.mySQLPodsWaitingForReadinessGates(ctx)
	if err != nil {
		return err
	}
	if podsWaiting {
		// Restart the mysql-operator to see if it will finish setting the readiness gates
		ctx.Log().Info("Restarting the mysql-operator to see if it will repair MySQL pods stuck waiting for readiness gates")

		operPod, err := getMySQLOperatorPod(ctx.Log(), ctx.Client())
		if err != nil {
			return fmt.Errorf("Failed restarting the mysql-operator to repair stuck MySQL pods: %v", err)
		}

		if err = ctx.Client().Delete(context.TODO(), operPod, &clipkg.DeleteOptions{}); err != nil {
			return err
		}

		// Clear the timer
		*c.LastTimeReadinessGateRepairStarted = time.Time{}
	}
	return nil
}

// mySQLPodsWaitingForReadinessGates - detect if there are MySQL pods stuck waiting for
// their readiness gates to be true.
func (c mysqlComponent) mySQLPodsWaitingForReadinessGates(ctx spi.ComponentContext) (bool, error) {
	if c.LastTimeReadinessGateRepairStarted.IsZero() {
		*c.LastTimeReadinessGateRepairStarted = time.Now()
		return false, nil
	}

	// Initiate repair only if time to wait period has been exceeded
	expiredTime := c.LastTimeReadinessGateRepairStarted.Add(5 * time.Minute)
	if time.Now().After(expiredTime) {
		// Check if the current not ready state is due to readiness gates not met
		ctx.Log().Debug("Checking if MySQL not ready due to pods waiting for readiness gates")

		selector := metav1.LabelSelectorRequirement{Key: mySQLComponentLabel, Operator: metav1.LabelSelectorOpIn, Values: []string{mySQLDComponentName}}
		podList := k8sready.GetPodsList(ctx.Log(), ctx.Client(), types.NamespacedName{Namespace: ComponentNamespace}, &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{selector}})
		if podList == nil || len(podList.Items) == 0 {
			return false, fmt.Errorf("Failed checking MySQL readiness gates, no pods found matching selector %s", selector.String())
		}

		for i := range podList.Items {
			pod := podList.Items[i]
			// Check if the readiness conditions have been met
			conditions := pod.Status.Conditions
			if len(conditions) == 0 {
				return false, fmt.Errorf("Failed checking MySQL readiness gates, no status conditions found for pod %s/%s", pod.Namespace, pod.Name)
			}
			readyCount := 0
			for _, condition := range conditions {
				for _, gate := range pod.Spec.ReadinessGates {
					if condition.Type == gate.ConditionType && condition.Status == v1.ConditionTrue {
						readyCount++
						continue
					}
				}
			}

			// All readiness gates must be true
			if len(pod.Spec.ReadinessGates) != readyCount {
				return true, nil
			}
		}
	}
	return false, nil
}

// getMySQLOperatorPod - return the mysql-operator pod
func getMySQLOperatorPod(log vzlog.VerrazzanoLogger, client clipkg.Client) (*v1.Pod, error) {
	operSelector := metav1.LabelSelectorRequirement{Key: "name", Operator: metav1.LabelSelectorOpIn, Values: []string{mysqloperator.ComponentName}}
	operPodList := k8sready.GetPodsList(log, client, types.NamespacedName{Namespace: mysqloperator.ComponentNamespace}, &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{operSelector}})
	if operPodList == nil || len(operPodList.Items) != 1 {
		return nil, fmt.Errorf("no pods found matching selector %s", operSelector.String())
	}
	return &operPodList.Items[0], nil
}