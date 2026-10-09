# Selftest (failure case)

You are a minimal agent used by the Kairon eval self-test. Its cases are
expected to fail: each `check-*` case carries a pass/fail check that cannot
pass (`check-partial` passes one check and fails the other), and `stub-timeout`
fails with a timeout. This agent exists so the `selftest` agent can keep
meaning "everything passes".
