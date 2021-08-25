# Flagger signed releases

Flagger releases published to GitHub Container Registry as multi-arch container images
are signed using [cosign](https://github.com/sigstore/cosign).

## Verify Flagger images

Install the [cosign](https://github.com/sigstore/cosign) CLI:

```sh
brew install sigstore/tap/cosign
```

Verify a Flagger release with cosign CLI:

```sh
cosign verify -key https://raw.githubusercontent.com/fluxcd/flagger/main/cosign/cosign.pub \
ghcr.io/fluxcd/flagger:1.13.0
```

Verify Flagger images before they get pulled on your Kubernetes clusters with [Kyverno](https://github.com/kyverno/kyverno/):

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: verify-flagger-image
  annotations:
    policies.kyverno.io/title: Verify Flagger Image
    policies.kyverno.io/category: Cosign
    policies.kyverno.io/severity: medium
    policies.kyverno.io/subject: Pod
    policies.kyverno.io/minversion: 1.4.2
spec:
  validationFailureAction: enforce
  background: false
  rules:
    - name: verify-image
      match:
        resources:
          kinds:
            - Pod
      verifyImages:
      - image: "ghcr.io/fluxcd/flagger:*"
        key: |-
          -----BEGIN PUBLIC KEY-----
          MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEST+BqQ1XZhhVYx0YWQjdUJYIG5Lt
          iz2+UxRIqmKBqNmce2T+l45qyqOs99qfD7gLNGmkVZ4vtJ9bM7FxChFczg==
          -----END PUBLIC KEY-----     
```
