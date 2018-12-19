---
description: Flagger is an Istio progressive delivery Kubernetes operator
---

# Introduction

[Flagger](https://github.com/stefanprodan/flagger) is a **Kubernetes** operator that automates the promotion of canary deployments using **Istio** routing for traffic shifting and **Prometheus** metrics for canary analysis. 

Flagger implements a control loop that gradually shifts traffic to the canary while measuring key performance indicators like HTTP requests success rate, requests average duration and pods health. Based on the **KPIs** analysis a canary is promoted or aborted and the analysis result is published to **Slack**.

Flagger takes a Kubernetes deployment and creates a series of objects \(Kubernetes [deployments](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/), ClusterIP [services](https://kubernetes.io/docs/concepts/services-networking/service/) and Istio [virtual services](https://istio.io/docs/reference/config/istio.networking.v1alpha3/#VirtualService)\) to drive the canary analysis and promotion.

![flagger-overview](https://raw.githubusercontent.com/stefanprodan/flagger/master/docs/diagrams/flagger-overview.png)



