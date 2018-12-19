# Flagger

Flagger is a Kubernetes operator that automates the promotion of canary deployments using Istio routing for traffic 
shifting and Prometheus metrics for canary analysis.

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance 
indicators like HTTP requests success rate, requests average duration and pods health. Based on the KPIs analysis 
a canary is promoted or aborted and the analysis result is published to Slack.

### For the install instructions and usage examples please see [docs.flagger.app](https://docs.flagger.app)

