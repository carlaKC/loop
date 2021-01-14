# Loop Client Release Notes
This file tracks release notes for the loop client. 

### Developers: 
* When new features are added to the repo, a short description of the feature should be added under the "Next Release" heading.
* This should be done in the same PR as the change so that our release notes stay in sync!

### Release Manager: 
* All of the items under the "Next Release" heading should be included in the release notes.
* As part of the PR that bumps the client version, cut everything below the 'Next Release' heading. 
* These notes can either be pasted in a temporary doc, or you can get them from the PR diff once it is merged. 
* The notes are just a guideline as to the changes that have been made since the last release, they can be updated.
* Once the version bump PR is merged and tagged, add the release notes to the tag on GitHub.

## Next release
- Fixed compile time compatibility with `lnd v0.12.0-beta`.

#### New Features
* If lnd is locked when the loop client starts up, it will wait for lnd to be 
  unlocked. Previous versions would exit with an error. 
* The rules used for autoloop have been relaxed to allow autoloop to dispatch
  swaps even if there are manually initiated swaps that are not limited to a
  single channel in progress. This change was made to allow autoloop to coexist
  with manual swaps. 
* The `SuggestSwaps` rpc call will now fail with a `FailedPrecondition` grpc
  error if no rules are configured for the autolooper. Previously the rpc would
  fail silently. 

#### Breaking Changes

#### Bug Fixes
