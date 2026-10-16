#!/bin/bash
# Harbor runs this from the task's working directory (/app) after the agent finishes.
chmod +x /tests/grade-* 2>/dev/null
exec /tests/grade-"$(uname -m)" --runner bun mean.test.ts median.test.ts
