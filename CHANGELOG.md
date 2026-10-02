# Changelog

All notable changes to Kairon will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**Note**: Historical entries below reflect the project's original name (kiro-krew) as they were written. The project was renamed to Kairon in October 2024.

---

# 1.0.0 (2026-06-24)


### Bug Fixes

* add allowedTools to architect and krew-lead configs ([ec89247](https://github.com/jbrinkman/kairon/commit/ec89247ff705a3630642eb00f322b7ee5cb6cc50))
* add frontmatter to SKILL.md and correct invocation syntax ([9ef9b40](https://github.com/jbrinkman/kairon/commit/9ef9b40dda22c205944fd1e0a9844f6fbab52652))
* add missing architect prompt and explicit agent naming ([01025d2](https://github.com/jbrinkman/kairon/commit/01025d20036a0cbde7ddcefe369712f3a12e103a))
* add missing krew-lead prompt and fix PID log confusion ([52cb31e](https://github.com/jbrinkman/kairon/commit/52cb31efd3b42be9cb3dd7a87922af32463d07e5))
* avoid holding manager lock during PR verification ([b57c5e3](https://github.com/jbrinkman/kairon/commit/b57c5e302f77048e8fec919b9480fcea44d95a19))
* avoid holding manager lock during PR verification ([d479505](https://github.com/jbrinkman/kairon/commit/d479505b9874f389988a331b96007a9624b38d8f))
* clarify agent state refresh after PR check ([013b1ea](https://github.com/jbrinkman/kairon/commit/013b1ea650fadb6cac42bfc4fbc401450d65b015))
* correct .gitignore entry for sessions folder ([011e3aa](https://github.com/jbrinkman/kairon/commit/011e3aaab3c52becec8e8314252be49e12f5c928))
* eval framework creates empty results folders with no logging output ([#124](https://github.com/jbrinkman/kairon/issues/124)) ([e413332](https://github.com/jbrinkman/kairon/commit/e413332b3a87bfcff770c31833c7aac1daa7122f))
* **eval:** add per-criterion deltas to eval diff output ([4919005](https://github.com/jbrinkman/kairon/commit/4919005c1e07dbbbf2241368973bc3ed076ae426))
* **eval:** ship captured outputs in test case templates ([db59dc5](https://github.com/jbrinkman/kairon/commit/db59dc5bf47684959fe93747f82bc38624997784))
* **eval:** skip LLM-judged criteria instead of returning fake midpoint scores ([4d7fc3c](https://github.com/jbrinkman/kairon/commit/4d7fc3c06d9719d7a54ae61d7fcc6474204c2531))
* **eval:** skip unknown deterministic criteria instead of awarding full credit ([6e4ac0e](https://github.com/jbrinkman/kairon/commit/6e4ac0efce7cb18eb0b620a8dc933ed3740f3949))
* **eval:** sync captured outputs from templates to project-level eval cases ([b9d07d4](https://github.com/jbrinkman/kairon/commit/b9d07d452fd7bf8f61472377fbc11a8ce88fc125))
* **eval:** verify file references with os.Stat instead of pattern matching ([d9785ed](https://github.com/jbrinkman/kairon/commit/d9785ed4921f991a2226ea520af97125ef320f05))
* extract platform-specific syscall usage for Windows compatibility ([a6a4a2e](https://github.com/jbrinkman/kairon/commit/a6a4a2eb67bf17a64b3a4f2f4373b742c092911a))
* improve PR description template in krew-lead prompt ([23c0147](https://github.com/jbrinkman/kairon/commit/23c0147c38bc0a587313210d58216dfcee479ea6))
* include commit hash in about overlay after update check ([#110](https://github.com/jbrinkman/kairon/issues/110)) ([ad71d7f](https://github.com/jbrinkman/kairon/commit/ad71d7f5670fdd8cc9f60c3f86767d27b5399976)), closes [#109](https://github.com/jbrinkman/kairon/issues/109)
* integrate label addition into planner agent workflow ([2247411](https://github.com/jbrinkman/kairon/commit/22474116fb114e7cb1e737c740bb42f680441c65)), closes [#17](https://github.com/jbrinkman/kairon/issues/17)
* minor config and docs fixes ([#1](https://github.com/jbrinkman/kairon/issues/1)) ([f47e0b7](https://github.com/jbrinkman/kairon/commit/f47e0b74a08686968dab1d5a26bf113a6a9df845))
* only log retry-file cleanup when file is removed ([72937d5](https://github.com/jbrinkman/kairon/commit/72937d5c9b0658adabfdd81a0a3cc673a7f30b96))
* parse PR check output as JSON ([eda7028](https://github.com/jbrinkman/kairon/commit/eda70288562741f271e485336c6d47b7239fbc95))
* remove extra closing brace causing compile error in TUI ([7980c42](https://github.com/jbrinkman/kairon/commit/7980c42c56d5dfebbb53df0ecba8e415493335e6))
* remove invalid sentinelFile field from agent JSON configs ([#81](https://github.com/jbrinkman/kairon/issues/81)) ([917265a](https://github.com/jbrinkman/kairon/commit/917265aae7fa2340fb7c8fe6bf4bec8bf64c18c9)), closes [#79](https://github.com/jbrinkman/kairon/issues/79)
* resolve label parsing bug and add observability logging ([69c5890](https://github.com/jbrinkman/kairon/commit/69c5890af9106dda81ed4ba765575a306951b336))
* resolve TUI prompt rendering and add planner guardrails ([6d77fed](https://github.com/jbrinkman/kairon/commit/6d77fedad8245a7e39e6f68bfb6f0b4c3bb3eead))
* revalidate agent state after PR check ([e7ba32e](https://github.com/jbrinkman/kairon/commit/e7ba32ee5901fbfa57b172a18460148ee613a2d4))
* spawn agents inside worktree to prevent work landing on main ([7d3c21c](https://github.com/jbrinkman/kairon/commit/7d3c21c8f1682ba683235ad4f0df87fccdb388b3))
* **tui:** resolve REPL unresponsiveness caused by log position never advancing ([e409541](https://github.com/jbrinkman/kairon/commit/e409541dade0004d64aa78afeb0c8fc4af1d4de4))
* **tui:** restore terminal input state after subprocess exit ([3bfb07d](https://github.com/jbrinkman/kairon/commit/3bfb07db625f454e6317e5b52527847928efb2e5)), closes [#7](https://github.com/jbrinkman/kairon/issues/7)
* use --body-file in planner prompt for safe multi-line issue creation ([d976e60](https://github.com/jbrinkman/kairon/commit/d976e607c2b81116cae9afc974d3c0a3e9aa8767))
* use Task variable for version:set to work in CI ([a596572](https://github.com/jbrinkman/kairon/commit/a59657210ee5178b848828d0b7abb28b60861e00))
* watcher should not auto-start and use correct kiro-cli flags ([f33779d](https://github.com/jbrinkman/kairon/commit/f33779d3658f273f62394ebc4a62bdbff80319cc))
* **watcher:** use case-insensitive comparison for issue state ([#96](https://github.com/jbrinkman/kairon/issues/96)) ([5e08f02](https://github.com/jbrinkman/kairon/commit/5e08f022f3cec44072493adba97d2a9608855ef3)), closes [#95](https://github.com/jbrinkman/kairon/issues/95)


* feat!: upgrade BubbleTea to v2.0.6 and all Charm dependencies ([938841b](https://github.com/jbrinkman/kairon/commit/938841b675884b223caa4cd691c6b189dcd6abef)), closes [#14](https://github.com/jbrinkman/kairon/issues/14)


### Features

* Add console view scrolling support ([#60](https://github.com/jbrinkman/kairon/issues/60)) ([0ce03a0](https://github.com/jbrinkman/kairon/commit/0ce03a09e489a29c9a46c5f5124118a1b8e2fff8)), closes [#57](https://github.com/jbrinkman/kairon/issues/57)
* Add help command support to kiro-krew CLI ([#35](https://github.com/jbrinkman/kairon/issues/35)) ([7f0f031](https://github.com/jbrinkman/kairon/commit/7f0f03167524728367bb19cdf34b75b18fe53a43))
* add per-issue agent logging and heartbeat status ([8b0ba65](https://github.com/jbrinkman/kairon/commit/8b0ba6580b5b7178f1e666b88b4d424a39428ea0))
* **config:** enable copilot review ([e4c5ade](https://github.com/jbrinkman/kairon/commit/e4c5adec73f0f3c6ba5c3585154e5f6c994f587c))
* **eval:** add per-agent evaluation framework with rubrics and cost tracking ([f1b76ff](https://github.com/jbrinkman/kairon/commit/f1b76ffde993e46af8b2400cb960d8be780afa1d)), closes [#10](https://github.com/jbrinkman/kairon/issues/10)
* **eval:** add rubrics and test cases for documenter and krew-lead agents ([3a694d1](https://github.com/jbrinkman/kairon/commit/3a694d19d7243b461babf8f0d38dfb1a32507068))
* execute validator agent after every task ([#2](https://github.com/jbrinkman/kairon/issues/2)) ([a10fd17](https://github.com/jbrinkman/kairon/commit/a10fd1738a251544f7730749e0529279728dd2d2))
* Kiro CLI 2.0 compatibility ([#7](https://github.com/jbrinkman/kairon/issues/7)) ([7111339](https://github.com/jbrinkman/kairon/commit/71113398c77c45fa63e1669873971a087f0a8952))
* **tui:** add plan command for interactive issue creation ([79e99dd](https://github.com/jbrinkman/kairon/commit/79e99dd3104507469d6223c49cfaee590a654ba5)), closes [#4](https://github.com/jbrinkman/kairon/issues/4)
* **tui:** implement modal dialog overlay for planning mode ([3b91c23](https://github.com/jbrinkman/kairon/commit/3b91c23362a928e2f7a835eac940d74faff76afa)), closes [#16](https://github.com/jbrinkman/kairon/issues/16)
* verify PR exists before marking issue as done ([fa406be](https://github.com/jbrinkman/kairon/commit/fa406be8e0a3df29aebc4d9919b43e8f4632a646)), closes [#23](https://github.com/jbrinkman/kairon/issues/23)


### Performance Improvements

* fix TUI lock contention causing sluggish input ([#93](https://github.com/jbrinkman/kairon/issues/93)) ([007c216](https://github.com/jbrinkman/kairon/commit/007c2166c7f6a045c28a65bc28212b10f96abfa7))


### BREAKING CHANGES

* requires Go 1.25+ due to bubbles/v2 dependency
* Requires Kiro CLI 2.0+ for skills support

* feat: enhance agent prompts and update gitignore

team-lead-prompt.md:
- Enforce stricter worktree isolation with mandatory pre-merge validation
- Require explicit worktree path in every subagent query
- Enforce git commits after each builder task
- Add location verification to catch files outside worktree

validator-prompt.md:
- Document shell access with autoAllowReadonly setting
- Clarify read-only command expectations

documenter.json:
- Add welcomeMessage for better UX

.gitignore:
- Add patterns for logs, .claude, app_docs, specs, test-results
- Fix agents/ pattern to only match root directory

* docs: update README for Kiro CLI 2.0 compatibility

- Update workflow diagrams to show single-message skill invocation
- Document skills vs prompts migration
- Update file structure to show .kiro/skills/ directory
- Change requirements from Kiro CLI 1.x to 2.0+
- Remove outdated 2.x incompatibility warning
- Update command reference for inline arguments
