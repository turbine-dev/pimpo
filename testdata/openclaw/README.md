# Routines OpenClaw and Hermes users run most

Traces for the compiler proof, one per routine that people report running most with OpenClaw and Hermes Agent (survey of 2026-09-28: the OpenClaw cron and heartbeat docs, the Hermes cron guide and user stories, awesome-openclaw-usecases, awesome-hermes-usecases, ClawHub install counts). `make.py` writes them; run the proof with

    go run ./cmd/proof -dir testdata/openclaw -out docs/proof/openclaw

| Trace | Routine | Source |
|---|---|---|
| oc-01 | Morning brief: weather, calendar, important mail, Todoist | both; the most mentioned |
| oc-02 | Urgent mail triage | OpenClaw heartbeat checklist |
| oc-03 | Yesterday's inbox to Slack | Hermes user stories |
| oc-04 | Weekly AI news from Hacker News | both; Hermes template |
| oc-05 | Meeting prep (copy of proof 07) | OpenClaw |
| oc-06 | Meetings in the next hour | OpenClaw |
| oc-07 | GitHub repo watcher to Discord | both; Hermes template |
| oc-08 | Weekly report: closed issues, overdue tasks | OpenClaw |
| oc-09 | Bitcoin moves over 3% | Hermes template |
| oc-10 | B3 quotes with a variation flag | both (earnings/dividend trackers) |
| oc-11 | arXiv papers on agents to Obsidian | both; Hermes template |
| oc-12 | Reddit top posts | OpenClaw |
| oc-13 | New YouTube videos of the day | OpenClaw |
| oc-14 | Newsletters digest and archive | OpenClaw |
| oc-15 | Filtered apartment listings | Hermes user stories |
| oc-16 | Daily journal in Obsidian | OpenClaw |
| oc-17 | Doors and windows left open (Home Assistant) | OpenClaw (self-healing home server) |
| oc-18 | Habit check-in question | OpenClaw |
| oc-19 | Price drop through a JSON API | Hermes pricing monitor |

Not covered, because Pimpo lacks the capability: one-shot reminders ("in 30 minutes"), reading web pages that are not JSON, a routine that waits for the owner's answer, chained routines, server metrics, publishing content.
