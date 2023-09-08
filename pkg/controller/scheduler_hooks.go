/*
Copyright 2020 The Flux authors

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

package controller

import (
	"fmt"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/canary"
)

func (c *Controller) runConfirmTrafficIncreaseHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmTrafficIncreaseHook {
			err := CallWebhook(*canary, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for traffic increase approval %s",
					canary.Name, canary.Namespace, webhook.Name)
				if !webhook.MuteAlert {
					c.alert(canary, "Canary traffic increase is waiting for approval.", false, flaggerv1.SeverityWarn)
				}
				return false
			}
			c.recordEventInfof(canary, "Confirm-traffic-increase check %s passed", webhook.Name)
		}
	}
	return true
}

func (c *Controller) runConfirmRolloutHooks(canary *flaggerv1.Canary, canaryController canary.Controller) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmRolloutHook {
			err := CallWebhook(*canary, canary.Status.Phase, webhook)
			if err != nil {
				if canary.Status.Phase != flaggerv1.CanaryPhaseWaiting {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseWaiting); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
					}
					c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for approval %s",
						canary.Name, canary.Namespace, webhook.Name)
					if !webhook.MuteAlert {
						c.alert(canary, "Canary is waiting for approval.", false, flaggerv1.SeverityWarn)
					}
				}
				return false
			}
			c.recordEventInfof(canary, "Confirm-rollout check %s passed", webhook.Name)
		}
	}
	return true
}

func (c *Controller) runConfirmPromotionHooks(canary *flaggerv1.Canary, canaryController canary.Controller) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmPromotionHook {
			err := CallWebhook(*canary, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				if canary.Status.Phase != flaggerv1.CanaryPhaseWaitingPromotion {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseWaitingPromotion); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
					}
					c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for promotion approval %s",
						canary.Name, canary.Namespace, webhook.Name)
					if !webhook.MuteAlert {
						c.alert(canary, "Canary promotion is waiting for approval.", false, flaggerv1.SeverityWarn)
					}
				} else {
					if err := canaryController.SetStatusIterations(canary, canary.GetAnalysis().Iterations-1); err != nil {
						c.recordEventWarningf(canary, "%v", err)
					}
				}
				return false
			} else {
				c.recordEventInfof(canary, "Confirm-promotion check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runPreRolloutHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.PreRolloutHook {
			err := CallWebhook(*canary, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement pre-rollout check %s failed %v",
					canary.Name, canary.Namespace, webhook.Name, err)
				return false
			} else {
				c.recordEventInfof(canary, "Pre-rollout check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runPostRolloutHooks(canary *flaggerv1.Canary, phase flaggerv1.CanaryPhase) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.PostRolloutHook {
			err := CallWebhook(*canary, phase, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Post-rollout hook %s failed %v", webhook.Name, err)
				return false
			} else {
				c.recordEventInfof(canary, "Post-rollout check %s passed", webhook.Name)
			}
		}
	}
	return true
}

func (c *Controller) runRollbackHooks(canary *flaggerv1.Canary, phase flaggerv1.CanaryPhase) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.RollbackHook {
			err := CallWebhook(*canary, phase, webhook)
			if err != nil {
				c.recordEventInfof(canary, "Rollback hook %s not signaling a rollback", webhook.Name)
			} else {
				c.recordEventWarningf(canary, "Rollback check %s passed", webhook.Name)
				return true
			}
		}
	}
	return false
}
