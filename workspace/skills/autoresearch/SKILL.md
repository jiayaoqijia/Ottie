# Autoresearch Skill

Autonomous experiment loop for codebase optimization, benchmarking, and research tasks.
Adapted from [Karpathy's autoresearch](https://github.com/karpathy/autoresearch) pattern.

## Core Loop

1. **Modify** — make one small, targeted change
2. **Run** — execute the experiment (build, test, benchmark)
3. **Measure** — compare results against baseline
4. **Keep/Discard** — only keep changes that improve the metric
5. **Repeat** — loop until interrupted or goal is met

## Rules

- One change at a time. Never batch multiple ideas into one experiment.
- Always measure before and after. No "I think this is better."
- Log every experiment to `experiments.tsv` (timestamp, description, metric_before, metric_after, kept).
- Revert immediately if the metric doesn't improve.
- Prefer simple changes. Only keep complexity that justifies itself with measurable gain.
- Never stop. After each experiment, pick the next most promising hypothesis.

## Usage

```
Activate the autoresearch skill. My goal is: [describe objective].
The metric is: [describe what to measure].
The baseline command is: [command to run].
```

## Experiment Log Format

```tsv
timestamp	description	metric_before	metric_after	kept	notes
2026-03-14T10:00:00Z	increase buffer size to 8k	42ms	38ms	yes	10% improvement
2026-03-14T10:05:00Z	switch to sync.Pool	38ms	41ms	no	added complexity, no gain
```

## References

- `references/program.md` — original autoresearch program prompt

## Scripts

- `scripts/run-experiment.sh` — timeout wrapper for running experiments safely
