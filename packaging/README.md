# Packaging

Manifests for the package managers, plus the script that regenerates them.

```sh
packaging/bump.sh v1.1.1
```

**This runs itself.** The release workflow regenerates the manifests and commits
them after each tag, because every value in them — including all ten hashes —
is derived from the release that was just published. There is no judgement in
it, so there is nothing for a person to review. Run it by hand only to
regenerate out of band. Every hash is read from that release's own
`checksums.txt`, so the manifests cannot disagree with what shipped. The files
are written out whole rather than patched, because editing a hash by its
position in a file is the kind of mistake that ships quietly.

## Publishing each one

**Homebrew** — needs a tap repository named `hammadsaedi/homebrew-tap`. Copy the
formula to `Formula/helikopter.rb` there and push:

```sh
brew install hammadsaedi/tap/helikopter
```

**Scoop** — needs a bucket repository, or submit to
[ScoopInstaller/Extras](https://github.com/ScoopInstaller/Extras). With your own
bucket:

```sh
scoop bucket add hammadsaedi https://github.com/hammadsaedi/scoop-bucket
scoop install helikopter
```

**winget** — open a pull request against
[microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs), copying the
`manifests/` tree into theirs. Validate first:

```sh
winget validate --manifest packaging/winget/manifests/h/hammadsaedi/helikopter/1.0.0
```

```sh
winget install hammadsaedi.helikopter
```

## Where this is going

The manifests in this directory are a staging area. What actually reaches
anyone is the tap repository, the bucket repository and the winget submission,
and none of those exist yet. Once the tap and bucket do, the release workflow
should push to them directly and these copies can go: a file that is generated,
committed and then never read is just somewhere for the truth to drift.

winget stays a submission, since it is a pull request against a repository
nobody here owns.

## What packaging does not fix

None of these avoid the Windows warning about an unidentified publisher. Smart
App Control checks for a signature when the program runs, not for how it was
delivered, so an unsigned binary is blocked whichever route installed it.

The fixes are to sign the Windows binaries with an Authenticode certificate —
[SignPath](https://signpath.io) is free for open source, and Azure Trusted
Signing is inexpensive — or to build from source, where nothing needs
verifying:

```sh
go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest
```
