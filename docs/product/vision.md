# DistroLearn Vision

## What

A browser-based learning platform for distributed systems. Learners read a concept,
then work it in a real, throwaway environment: a sandbox lab that boots in seconds,
exposes a terminal and services, verifies the learner's work, and is destroyed when
they are done.

## Who

- **Learner** — student, bootcamp participant, or engineer upskilling. Wants to
  touch a real cluster, not read about one.
- **Instructor** — university or corporate trainer. Needs a cohort, assignments, and
  evidence that the work happened.
- **Institution** — the buying unit. Needs tenant isolation, roster management, and a
  data-residency answer for EU data protection review.

## Why

Distributed systems are taught with diagrams and toy simulations. The concepts that
actually matter (consensus, replication, partial failure, rebalancing) only show up
when a node dies mid-request and you have to notice. Existing platforms either stop at
the diagram or hand students an image they install locally, which excludes anyone
without a spare machine and expires the moment they close the laptop.

## Scale target

- 50k monthly active users
- 5k concurrent labs
- Multi-tenant by institution from day one
- EU data residency as a tenant-selectable option

## Measures

| Measure | Target |
|---|---|
| Weekly active learners | 25k of 50k MAU |
| Concurrent labs | 5k peak, sustained |
| Lab boot time to usable shell | p50 < 10s, p95 < 20s |
| Lab completion (started → verified) | > 55% |
| Learner-reported "would keep using" | > 70% |

## Non-goals for now

Not a general-purpose course marketplace, not a certification authority, and not a
replacement for a university curriculum. Depth on a narrow set of distributed-systems
topics beats breadth across all of software engineering.
