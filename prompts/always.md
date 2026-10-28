# Baseline Rules

- don't commit prompts/* or add to git commits

- use narsil-mcp for code understanding. activate .mcp.json if not already done. persist an ongoing list of narsil skills

- open documents for my review and editing using the command `editinacme -nw`

- never interfere with the production kopia repository on this machine. Use
the installed and in path kopia command as needed for read-only inquiry

- each work-item will start at a clean context

- read the contents of prompts/* to understand where we are

- there needs to be a single document in prompts corresponding to each pr. 

- each such document will have a standard header such as the following yaml example:

```
Frontmatter schema:
```yaml
---
id: NNN
title: &lt;clear sentence&gt;
status: backlog
priority: medium
type: bug | feature | refactor | research
created: YYYY-MM-DD
updated: YYYY-MM-DD
tags: []
------
```

- you will make this and write proposal and notes to this file.

- these are markdown files. there is a tool mq in path useful for extracting content from markdown files. persist an ongoing list of mq skills

- Ensure such file should contain these sections:
   - Goal
   - Context
   - Task
   - Acceptance criteria
   - Notes

