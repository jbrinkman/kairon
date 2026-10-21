We want a proper reporting feature for Kairon. Here is what we need, all of it, as one piece of work:

- Generate a weekly report of agent runs per repository, with totals for succeeded, failed and retried runs.
- Let each team pick which repositories and which labels go into their report.
- Support PDF, HTML and CSV output, and let teams choose the format.
- Send the report by email to a distribution list that each team maintains, and also post a summary to Slack.
- Schedule reports daily, weekly or monthly, at a time and time zone each team sets.
- Add a web dashboard with live charts for run duration, failure rate and retry rate, with filters by repository, label and date range.
- Add sign-in for the dashboard and report-specific permissions, so only members of a team see that team's reports.
- Keep generated reports for a retention period each team can configure, and delete them afterwards.
- Let people compare two reporting periods side by side.
- Provide an admin screen to manage teams, distribution lists and schedules.
- Migrate the existing CSV export so it uses the new reporting engine.
- Document all of it for users and operators.

There is no existing design for any of this. Teams will not agree on defaults, so please do not assume them.
