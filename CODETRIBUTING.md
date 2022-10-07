# Contributing Guidelines

The Virtual-Kubelet azure-aci provider accepts contributions via GitHub pull requests. This document outlines the process to help get your contribution accepted.

## Support Channels

This is an open source project and as such no formal support is available.
However, like all good open source projects we do offer "best effort" support
through [github issues](https://github.com/virtual-kubelet/azure-aci).

Before opening a new issue or submitting a new pull request, it's helpful to
search the project - it's likely that another user has already reported the
issue you're facing, or it's a known issue that we're already aware of.

## Issues

Issues are used as the primary method for tracking anything to do with the
Virtual-Kubelet azure-aci provider.

### Issue Lifecycle

All issue types follow the same general lifecycle.

1. Issue creation
2. Triage
    - The maintainer(s) in charge of triaging will apply the proper labels for the
      issue. This includes labels for priority, type, and metadata. If additional
      labels are needed in the future, we will add them.
    - If needed, clean up the title to succinctly and clearly state the issue.
      Also ensure that proposals are prefaced with "Proposal".
    - Add the issue to the correct milestone. If any questions come up, don't
      worry about adding the issue to a milestone until the questions are
      answered.
    - We attempt to do this process at least once per work day.
3. Discussion
    - "kind/feature" and "bug" issues should be connected to the PR that resolves it.
    - Whoever is working on a "kind/feature" or "bug" issue (whether a maintainer or
      someone from the community), should either assign the issue to themselves or
      make a comment in the issue saying that they are taking it.
    - "Proposal" and "Question" issues should remain open until they are
      either resolved or have remained inactive for more than 30 days. This will
      help keep the issue queue to a manageable size and reduce noise.
4. Issue closure

## How to Contribute a Patch

1. Fork the repository, develop and test your code changes. You can use the following command to clone your fork to your local

```shell
git clone https://github.com/<YOUR_GITHUB_ALIAS>/azure-aci.git
cd azure-aci

# add the azure-aci as the upstream
git remote add upstream https://github.com/virtual-kubelet/azure-aci.git
```

2. Submit a pull request.

## Submission and Review guidelines

We welcome and appreciate everyone to submit and review changes. Here are some guidelines to follow for help ensure
a successful contribution experience.

Please note these are general guidelines, and while they are a good starting point, they are not specifically rules.
If you have a question about something, feel free to ask:

- [#virtual-kubelet](https://kubernetes.slack.com/archives/C8YU1QP8W) on Kubernetes Slack
- [virtualkubelet-dev@lists.cncf.io](mailto:virtualkubelet-dev@lists.cncf.io)
- GitHub Issues

#### Use context.Context

Probably if it is a public/exported API, it should take a `context.Context`. Even if it doesn't need one today, it may
need it tomorrow, and then we have a breaking API change.

We use `context.Context` for storing loggers, tracing spans, and cancellation all across the project. Better safe
than sorry: add a `context.Context`.

#### Errors

Callers can't handle errors if they don't know what the error is, so make sure they can figure that out.
We use a package `errdefs` to define the types of errors we currently look out for. We do not typically look for
concrete error types, so check out `errdefs` and see if there is already an error type in there for your needs, or even
create a new one.

#### Testing

Ideally all behavior would be tested, in practice this is not the case. Unit tests are great, and fast. There is also
an end-to-end test suite for testing the overall behavior of the system. Please add tests. This is also a great place
to get started if you are new to the codebase.

## Code of conduct

Virtual-Kubelet azure-aci provider follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md) and [Microsoft Code of Conduct](CODE_OF_CONDUCT.md)