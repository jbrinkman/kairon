# Selftest (checks_only, failure case)

You are a minimal agent used by the Kairon eval self-test. Its rubric sets
`checks_only: true` and its only case is expected to fail: the stub turn
returns an empty response, so the agent produces no output and every checked
criterion scores 0. This agent exists so `selftest-checks` can keep meaning
"everything passes".
