# Releasing

Two ways, and they run the same checks.

**From a terminal**

```sh
make release VERSION=v1.2.3
```

**From GitHub** — Actions → *release* → *Run workflow* → type the version.

The browser path runs `scripts/release.sh` too, so a release cut from a phone
cannot skip a check that one cut from a laptop would have failed on. Nothing
after the tag needs a human either way.

## What happens

1. The script refuses unless you are on `main`, the tree is clean, `main`
   matches `origin/main`, the tag is new, and the tests, `gofmt` and `go vet`
   all pass.
2. It prints what has landed since the last tag, and says so if the changes
   look like they deserve a minor rather than a patch.
3. It tags and pushes.
4. The release workflow builds ten targets, generates `checksums.txt` and
   publishes them.
5. A second job regenerates the package manifests from that release's own
   checksums and commits them.

## Choosing the number

The rule that matters here: **anything a user can newly type or press is a
minor.**

| | |
| --- | --- |
| patch — `v1.1.1` | fixes only, no new surface |
| minor — `v1.2.0` | a new command, flag or key |
| major — `v2.0.0` | something that used to work no longer does |

v1.1.0 was a minor because it added `helikopter update` and the `w` key.
v1.1.1 was a patch because it only fixed the clock.

Calling a feature a patch is not a harmless simplification: someone pinned to
`~1.1.1` never receives it.

## Afterwards

The workflow does the rest, but two things are worth checking by hand, because
CI cannot:

```sh
curl -fsSL https://hammadsaedi.github.io/helikopter/install.sh | sh
helikopter update            # from the previous version, confirm it upgrades
```

## If a release is bad

Tags are not reusable once pushed — anyone who has already fetched has the old
one. Cut a new patch rather than moving the tag. Deleting a published release
breaks `curl | sh` for anyone mid-install, and breaks `helikopter update` for
everyone still on an older version, since both resolve through
`/releases/latest`.
