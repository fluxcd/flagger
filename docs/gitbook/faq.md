# Frequently asked questions

**Can Flagger be part of my integration tests?**
> Yes, Flagger supports webhooks to do integration testing.

**What if I only want to target beta testers?**
> That's a feature in Flagger, not in App Mesh. It's on the App Mesh roadmap.

**When do I use A/B testing when Canary?**
> One advantage of using A/B testing is that each version remains separated and routes aren't mixed.
>
> Using a Canary deployment can lead to behaviour like this one observed by a
> user:
>
> [..] during a canary deployment of our nodejs app, the version that is being served <50% traffic reports mime type mismatch errors in the browser (js as "text/html")
> When the deployment Passes/ Fails (doesn't really matter) the version that stays alive works as expected. If anyone has any tips or direction I would greatly appreciate it. Even if its as simple as I'm looking in the wrong place. Thanks in advance!
>
> The issue was that we were not maintaining session affinity while serving files for our frontend. Which resulted in any redirects or refreshes occasionally returning a mismatched app.*.js file (generated from vue)
>
> Read up on [A/B testing](https://docs.flagger.app/usage/ab-testing).

