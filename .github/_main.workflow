workflow "Publish Helm charts" {
  on = "push"
  resolves = ["helm-push"]
}

action "helm-lint" {
  uses = "stefanprodan/gh-actions/helm@master"
  args = ["lint charts/*"]
}

action "helm-push" {
  needs = ["helm-lint"]
  uses = "stefanprodan/gh-actions/helm-gh-pages@master"
  args = ["charts/*","https://flagger.app"]
  secrets = ["GITHUB_TOKEN"]
}

