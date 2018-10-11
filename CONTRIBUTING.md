# How to Contribute

Flagger is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub
pull requests. This document outlines some of the conventions on development
workflow, commit message formatting, contact points and other resources to make
it easier to get your contribution accepted.

We gratefully welcome improvements to documentation as well as to code.

## Certificate of Origin

By contributing to this project you agree to the Developer Certificate of
Origin (DCO). This document was created by the Linux Kernel community and is a
simple statement that you, as a contributor, have the legal right to make the
contribution.

## Chat

The project uses Slack: To join the conversation, simply join the
[Weave community](https://slack.weave.works/) Slack workspace.

## Getting Started

- Fork the repository on GitHub
- If you want to contribute as a developer, continue reading this document for further instructions
- If you have questions, concerns, get stuck or need a hand, let us know
  on the Slack channel. We are happy to help and look forward to having
  you part of the team. No matter in which capacity.
- Play with the project, submit bugs, submit pull requests!

## Contribution workflow

This is a rough outline of how to prepare a contribution:

- Create a topic branch from where you want to base your work (usually branched from master).
- Make commits of logical units.
- Make sure your commit messages are in the proper format (see below).
- Push your changes to a topic branch in your fork of the repository.
- If you changed code:
  - add automated tests to cover your changes
- Submit a pull request to the original repository.

## Acceptance policy

These things will make a PR more likely to be accepted:

- a well-described requirement
- new code and tests follow the conventions in old code and tests
- a good commit message (see below)
- All code must abide [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Names should abide [What's in a name](https://talks.golang.org/2014/names.slide#1)
- Code must build on both Linux and Darwin, via plain `go build`
- Code should have appropriate test coverage and tests should be written
  to work with `go test`

In general, we will merge a PR once one maintainer has endorsed it.
For substantial changes, more people may become involved, and you might
get asked to resubmit the PR or divide the changes into more than one PR.

### Format of the Commit Message

For Flux we prefer the following rules for good commit messages:

- Limit the subject to 50 characters and write as the continuation
  of the sentence "If applied, this commit will ..."
- Explain what and why in the body, if more than a trivial change;
  wrap it at 72 characters.

The [following article](https://chris.beams.io/posts/git-commit/#seven-rules)
has some more helpful advice on documenting your work.

This doc is adapted from the [Weaveworks Flux](https://github.com/weaveworks/flux/blob/master/CONTRIBUTING.md)
