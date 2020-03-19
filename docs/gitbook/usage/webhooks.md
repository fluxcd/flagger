# Webhooks

The canary analysis can be extended with webhooks. Flagger will call each webhook URL and
determine from the response status code (HTTP 2xx) if the canary is failing or not.

There are several types of hooks:
* **confirm-rollout** hooks are executed before scaling up the canary deployment and can be used for manual approval.
The rollout is paused until the hook returns a successful HTTP status code.
* **pre-rollout** hooks are executed before routing traffic to canary. 
The canary advancement is paused if a pre-rollout hook fails and if the number of failures reach the 
threshold the canary will be rollback.
* **rollout** hooks are executed during the analysis on each iteration before the metric checks. 
If a rollout hook call fails the canary advancement is paused and eventfully rolled back.
* **confirm-promotion** hooks are executed before the promotion step.
The canary promotion is paused until the hooks return HTTP 200.
While the promotion is paused, Flagger will continue to run the metrics checks and rollout hooks.
* **post-rollout** hooks are executed after the canary has been promoted or rolled back. 
If a post rollout hook fails the error is logged.
* **rollback** hooks are executed while a canary deployment is in either Progressing or Waiting status.
This provides the ability to rollback during analysis or while waiting for a confirmation. If a rollback hook
returns a successful HTTP status code, Flagger will stop the analysis and mark the canary release as failed.
* **event** hooks are executed every time Flagger emits a Kubernetes event. When configured,
every action that Flagger takes during a canary deployment will be sent as JSON via an HTTP POST request.

Spec:

```yaml
  analysis:
    webhooks:
      - name: "start gate"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/approve
      - name: "helm test"
        type: pre-rollout
        url: http://flagger-helmtester.flagger/
        timeout: 3m
        metadata:
          type: "helmv3"
          cmd: "test podinfo -n test"
      - name: "load test"
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          cmd: "hey -z 1m -q 5 -c 2 http://podinfo-canary.test:9898/"
      - name: "promotion gate"
        type: confirm-promotion
        url: http://flagger-loadtester.test/gate/approve
      - name: "notify"
        type: post-rollout
        url: http://telegram.bot:8080/
        timeout: 5s
        metadata:
          some: "message"
      - name: "rollback gate"
        type: rollback
        url: http://flagger-loadtester.test/rollback/check
      - name: "send to Slack"
        type: event
        url: http://event-recevier.notifications/slack
```

> **Note** that the sum of all rollout webhooks timeouts should be lower than the analysis interval. 

Webhook payload (HTTP POST):

```json
{
    "name": "podinfo",
    "namespace": "test",
    "phase": "Progressing", 
    "metadata": {
        "test":  "all",
        "token":  "16688eb5e9f289f1991c"
    }
}
```

Response status codes:

* 200-202 - advance canary by increasing the traffic weight
* timeout or non-2xx - halt advancement and increment failed checks

On a non-2xx response Flagger will include the response body (if any) in the failed checks log and Kubernetes events.

Event payload (HTTP POST):

```json
{
  "name": "string (canary name)",
  "namespace": "string (canary namespace)",
  "phase": "string (canary phase)",
  "metadata": {
    "eventMessage": "string (canary event message)",
    "eventType": "string (canary event type)",
    "timestamp": "string (unix timestamp ms)"
  }
}
```

The event receiver can create alerts based on the received phase
(possible values: ` Initialized`, `Waiting`, `Progressing`, `Promoting`, `Finalising`, `Succeeded` or `Failed`).

### Load Testing

For workloads that are not receiving constant traffic Flagger can be configured with a webhook, 
that when called, will start a load test for the target workload.
If the target workload doesn't receive any traffic during the canary analysis, 
Flagger metric checks will fail with "no values found for metric request-success-rate".

Flagger comes with a load testing service based on [rakyll/hey](https://github.com/rakyll/hey) 
that generates traffic during analysis when configured as a webhook.

![Flagger Load Testing Webhook](https://raw.githubusercontent.com/weaveworks/flagger/master/docs/diagrams/flagger-load-testing.png)

First you need to deploy the load test runner in a namespace with sidecar injection enabled:

```bash
kubectl apply -k github.com/weaveworks/flagger//kustomize/tester
```

Or by using Helm:

```bash
helm repo add flagger https://flagger.app

helm upgrade -i flagger-loadtester flagger/loadtester \
--namespace=test \
--set cmd.timeout=1h
```

When deployed the load tester API will be available at `http://flagger-loadtester.test/`. 

Now you can add webhooks to the canary analysis spec:

```yaml
webhooks:
  - name: load-test-get
    url: http://flagger-loadtester.test/
    timeout: 5s
    metadata:
      type: cmd
      cmd: "hey -z 1m -q 10 -c 2 http://podinfo-canary.test:9898/"
  - name: load-test-post
    url: http://flagger-loadtester.test/
    timeout: 5s
    metadata:
      type: cmd
      cmd: "hey -z 1m -q 10 -c 2 -m POST -d '{test: 2}' http://podinfo-canary.test:9898/echo"
```

When the canary analysis starts, Flagger will call the webhooks and the load tester will run the `hey` commands 
in the background, if they are not already running. This will ensure that during the 
analysis, the `podinfo-canary.test` service will receive a steady stream of GET and POST requests.

If your workload is exposed outside the mesh you can point `hey` to the 
public URL and use HTTP2.

```yaml
webhooks:
  - name: load-test-get
    url: http://flagger-loadtester.test/
    timeout: 5s
    metadata:
      type: cmd
      cmd: "hey -z 1m -q 10 -c 2 -h2 https://podinfo.example.com/"
```

For gRPC services you can use [bojand/ghz](https://github.com/bojand/ghz) which is a similar tool to Hey but for gRPC:

```yaml
webhooks:
  - name: grpc-load-test
    url: http://flagger-loadtester.test/
    timeout: 5s
    metadata:
      type: cmd
      cmd: "ghz -z 1m -q 10 -c 2 --insecure podinfo.test:9898"
```

`ghz` uses reflection to identify which gRPC method to call. If you do not wish to enable reflection for your gRPC service you can implement a standardized health check from the [grpc-proto](https://github.com/grpc/grpc-proto) library. To use this [health check schema](https://github.com/grpc/grpc-proto/blob/master/grpc/health/v1/health.proto) without reflection you can pass a parameter to `ghz` like this

```yaml
webhooks:
  - name: grpc-load-test-no-reflection
    url: http://flagger-loadtester.test/
    timeout: 5s
    metadata:
      type: cmd
      cmd: "ghz --insecure --proto=/tmp/ghz/health.proto --call=grpc.health.v1.Health/Check podinfo.test:9898"
```

The load tester can run arbitrary commands as long as the binary is present in the container image.
For example if you you want to replace `hey` with another CLI, you can create your own Docker image:

```dockerfile
FROM weaveworks/flagger-loadtester:<VER>

RUN curl -Lo /usr/local/bin/my-cli https://github.com/user/repo/releases/download/ver/my-cli \
    && chmod +x /usr/local/bin/my-cli
```

### Load Testing Delegation

The load tester can also forward testing tasks to external tools, by now [nGrinder](https://github.com/naver/ngrinder)
is supported.

To use this feature, add a load test task of type 'ngrinder' to the canary analysis spec:

```yaml
webhooks:
  - name: load-test-post
    url: http://flagger-loadtester.test/
    timeout: 5s
    metadata:
      # type of this load test task, cmd or ngrinder
      type: ngrinder
      # base url of your nGrinder controller server
      server: http://ngrinder-server:port
      # id of the test to clone from, the test must have been defined.
      clone: 100
      # user name and base64 encoded password to authenticate against the nGrinder server
      username: admin
      passwd: YWRtaW4=
      # the interval between between nGrinder test status polling, default to 1s
      pollInterval: 5s
```
When the canary analysis starts, the load tester will initiate a [clone_and_start request](https://github.com/naver/ngrinder/wiki/REST-API-PerfTest)
to the nGrinder server and start a new performance test. the load tester will periodically poll the nGrinder server
for the status of the test, and prevent duplicate requests from being sent in subsequent analysis loops.

### Integration Testing

Flagger comes with a testing service that can run Helm tests, Bats tests or Concord tests when configured as a webhook.

Deploy the Helm test runner in the `kube-system` namespace using the `tiller` service account:

```bash
helm repo add flagger https://flagger.app

helm upgrade -i flagger-helmtester flagger/loadtester \
--namespace=kube-system \
--set serviceAccountName=tiller
```

When deployed the Helm tester API will be available at `http://flagger-helmtester.kube-system/`. 

Now you can add pre-rollout webhooks to the canary analysis spec:

```yaml
  analysis:
    webhooks:
      - name: "smoke test"
        type: pre-rollout
        url: http://flagger-helmtester.kube-system/
        timeout: 3m
        metadata:
          type: "helm"
          cmd: "test {{ .Release.Name }} --cleanup"
```

When the canary analysis starts, Flagger will call the pre-rollout webhooks before routing traffic to the canary.
If the helm test fails, Flagger will retry until the analysis threshold is reached and the canary is rolled back.

If you are using Helm v3, you'll have to create a dedicated service account and add the release namespace to the test command:

```yaml
  analysis:
    webhooks:
      - name: "smoke test"
        type: pre-rollout
        url: http://flagger-helmtester.kube-system/
        timeout: 3m
        metadata:
          type: "helmv3"
          cmd: "test {{ .Release.Name }} --timeout 3m -n {{ .Release.Namespace }}"
```

As an alternative to Helm you can use the [Bash Automated Testing System](https://github.com/bats-core/bats-core) to run your tests. 

```yaml
  analysis:
    webhooks:
      - name: "acceptance tests"
        type: pre-rollout
        url: http://flagger-batstester.default/
        timeout: 5m
        metadata:
          type: "bash"
          cmd: "bats /tests/acceptance.bats"
```

Note that you should create a ConfigMap with your Bats tests and mount it inside the tester container.

You can also configure the test runner to start a [Concord](https://concord.walmartlabs.com/) process.

```yaml
  analysis:
    webhooks:
      - name: "concord integration test"
        type: pre-rollout
        url: http://flagger-concordtester.default/
        timeout: 60s
        metadata:
          type: "concord"
          org: "your-concord-org"
          project: "your-concord-project"
          repo: "your-concord-repo"
          entrypoint: "your-concord-entrypoint"
          apiKeyPath: "/tmp/concord-api-key"
          endpoint: "https://canary-endpoint/"
          pollInterval: "5"
          pollTimeout: "60"
```

`org`, `project`, `repo` and `entrypoint` represents where your test process runs in Concord. 
In order to authenticate to Concord, you need to set `apiKeyPath` to a path of a file containing a valid Concord API key
 on the `flagger-helmtester` container. This can be done via mounting a Kubernetes secret in the tester's Deployment. 
`pollInterval` represents the interval in seconds the web-hook will call Concord to see if the process has finished (Default is 5s). 
`pollTimeout` represents the time in seconds the web-hook will try to call Concord before timing out (Default is 30s). 

### Manual Gating

For manual approval of a canary deployment you can use the `confirm-rollout` and `confirm-promotion` webhooks. 
The confirmation rollout hooks are executed before the pre-rollout hooks. 
Flagger will halt the canary traffic shifting and analysis until the confirm webhook returns HTTP status 200.

For manual rollback of a canary deployment you can use the `rollback` webhook.  The rollback hook will be called 
during the analysis and confirmation states.  If a rollback webhook returns a successful HTTP status code, Flagger 
will shift all traffic back to the primary instance and fail the canary. 

Manual gating with Flagger's tester:

```yaml
  analysis:
    webhooks:
      - name: "gate"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/halt
```

The `/gate/halt` returns HTTP 403 thus blocking the rollout. 

If you have notifications enabled, Flagger will post a message to Slack or MS Teams if a canary rollout is waiting for approval.

Change the URL to `/gate/approve` to start the canary analysis:

```yaml
  analysis:
    webhooks:
      - name: "gate"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/approve
```

Manual gating can be driven with Flagger's tester API. Set the confirmation URL to `/gate/check`:

```yaml
  analysis:
    webhooks:
      - name: "ask for confirmation"
        type: confirm-rollout
        url: http://flagger-loadtester.test/gate/check
```

By default the gate is closed, you can start or resume the canary rollout with:

```bash
kubectl -n test exec -it flagger-loadtester-xxxx-xxxx sh

curl -d '{"name": "podinfo","namespace":"test"}' http://localhost:8080/gate/open 
```

You can pause the rollout at any time with:

```bash
curl -d '{"name": "podinfo","namespace":"test"}' http://localhost:8080/gate/close 
```

If a canary analysis is paused the status will change to waiting:

```bash
kubectl get canary/podinfo

NAME      STATUS        WEIGHT
podinfo   Waiting       0
```

The `confirm-promotion` hook type can be used to manually approve the canary promotion.
While the promotion is paused, Flagger will continue to run the metrics checks and load tests.

```yaml
  analysis:
    webhooks:
      - name: "promotion gate"
        type: confirm-promotion
        url: http://flagger-loadtester.test/gate/halt
```

The `rollback` hook type can be used to manually rollback the canary promotion.  As with gating, rollbacks can be driven 
with Flagger's tester API by setting the rollback URL to `/rollback/check`  

```yaml
  analysis:
    webhooks:
      - name: "rollback"
        type: rollback
        url: http://flagger-loadtester.test/rollback/check
```     

By default rollback is closed, you can rollback a canary rollout with:

```bash
kubectl -n test exec -it flagger-loadtester-xxxx-xxxx sh

curl -d '{"name": "podinfo","namespace":"test"}' http://localhost:8080/rollback/open 
```

You can close the rollback with:

```bash
curl -d '{"name": "podinfo","namespace":"test"}' http://localhost:8080/rollback/close 
```

If you have notifications enabled, Flagger will post a message to Slack or MS Teams if a canary has been rolled back.
