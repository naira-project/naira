# Integration tests

The core idea of an integration test
is to run the component under test in a realistic way,
interacting with realistic neighbor components in concrete versions.
The tests are not intended to be exhaustive
(this is expected to be covered by unit tests,
which can use mocks, stubs, and fake responses for fast turnaround time
allowing cheaply and deterministically verifying
many edge cases and complex regression scenarios).
Instead, an integration test is more of a sanity check,
verifying that a *real* build of the tested component (with no shortcuts)
can *really* interact with *real* APIs of its realistic neighbors.
In other words, that the mocks used in unit tests are not lying,
accidentally simplifying things too much or missing some important nuances.

Also, integration test is intended to establish compatibility with specific,
concrete versions of neighbor components in basic optimistic scenarios.
This should help us report precise compatibility promises
and catch regressions vs. upgraded versions of neighbor components.

Note: an integration test is *not* a test of the *neighbor components* themselves.
It *must assume* that the neighbor components are working correctly.
Also, the test can, and should, use shortcuts and simplifications
*in the neighbor components*
(in order to make the test faster, more reliable, and cheaper),
as long as it still uses *real* neighbor components,
and as long as the shortcuts don't impact the APIs and behaviors
of the neighbor components
in ways that would be relevant to the component under test.
