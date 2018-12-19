# Alerting

### Slack

Flagger can be configured to send Slack notifications:

```bash
helm upgrade -i flagger flagger/flagger \
--namespace=istio-system \
--set slack.url=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK \
--set slack.channel=general \
--set slack.user=flagger
```

Once configured with a Slack incoming **webhook**, Flagger will post messages when a canary deployment has been initialised, when a new revision has been detected and if the canary analysis failed or succeeded.

![flagger-slack](https://raw.githubusercontent.com/stefanprodan/flagger/master/docs/screens/slack-canary-notifications.png)

A canary deployment will be rolled back if the progress deadline exceeded or if the analysis reached the maximum number of failed checks:

![flagger-slack-errors](https://raw.githubusercontent.com/stefanprodan/flagger/master/docs/screens/slack-canary-failed.png)

### Prometheus Alert Manager

Besides Slack, you can use Alertmanager to trigger alerts when a canary deployment failed:

```yaml
  - alert: canary_rollback
    expr: flagger_canary_status > 1
    for: 1m
    labels:
      severity: warning
    annotations:
      summary: "Canary failed"
      description: "Workload {{ $labels.name }} namespace {{ $labels.namespace }}"
```

