# Vision

## What

A learner reads a concept, then works it in a throwaway environment. A sandbox lab
boots in seconds, exposes a terminal, verifies the work, and is destroyed when done.

## Who

- **Learner.** Student, bootcamp participant, or engineer upskilling.
- **Instructor.** Trainer. Needs a cohort and evidence the work happened.
- **Institution.** The buying unit. Needs tenant isolation, a roster, and a
  data-residency answer for EU review.

## Why

Consensus, replication, partial failure, and rebalancing only show up when a node dies
mid-request and someone notices. Existing platforms stop at the diagram, or hand
students an image to install locally, excluding anyone without a spare machine.

## Scale and measures

Multi-tenant by institution from day one, EU residency tenant-selectable.

| Measure | Target |
|---|---|
| Monthly active users | 50k |
| Weekly active learners | 25k |
| Concurrent labs | 5k peak (ADR 0002) |
| Lab boot to usable shell | p50 < 10s, p95 < 20s |
| Lab completion, started to verified | > 55% |
| "Would keep using" | > 70% |

## Non-goals

Not a course marketplace, certification authority, or university curriculum. Depth on a
narrow set of topics beats breadth.
