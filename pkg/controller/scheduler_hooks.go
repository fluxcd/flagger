package controller

import (
	"fmt"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
)

func (c *Controller) runConfirmRolloutHooks(canary *flaggerv1.Canary, canaryController canary.Controller) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmRolloutHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				if canary.Status.Phase != flaggerv1.CanaryPhaseWaiting {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseWaiting); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
					}
					c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for approval %s",
						canary.Name, canary.Namespace, webhook.Name)
					c.alert(canary, "Canary is waiting for approval.", false, flaggerv1.SeverityWarn)
				}
				return false
			} else {
				if canary.Status.Phase == flaggerv1.CanaryPhaseWaiting {
					if err := canaryController.SetStatusPhase(canary, flaggerv1.CanaryPhaseProgressing); err != nil {
						c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).Errorf("%v", err)
						return false
					}
					c.recordEventInfof(canary, "Confirm-rollout check %s passed", webhook.Name)
					return false
				}
			}
		}
	}
	return true
}

func (c *Controller) runConfirmPromotionHooks(canary *flaggerv1.Canary) bool {
	for _, webhook := range canary.GetAnalysis().Webhooks {
		if webhook.Type == flaggerv1.ConfirmPromotionHook {
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
			if err != nil {
				c.recordEventWarningf(canary, "Halt %s.%s advancement waiting for promotion approval %s",
					canary.Name, canary.Namespace, webhook.Name)
				c.alert(canary, "Canary promotion is waiting for approval.", false, flaggerv1.SeverityWarn)
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
			err := CallWebhook(canary.Name, canary.Namespace, flaggerv1.CanaryPhaseProgressing, webhook)
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
			err := CallWebhook(canary.Name, canary.Namespace, phase, webhook)
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
			err := CallWebhook(canary.Name, canary.Namespace, phase, webhook)
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
