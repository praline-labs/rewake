#!/bin/sh
# The commit-msg check of lefthook.yml: AGENTS.md, "Commits" and "Code" — a one-line
# subject, then only trailer lines, with both Co-Authored-By trailers. Takes the path
# of the message file. The trailers are matched by the agent's name and an address,
# not by the model in the parentheses, so a model change needs no edit here.
set -u

file=${1:?usage: commit-msg-check.sh <message file>}

# Git's own comment lines are not part of the message.
message=$(grep -v '^#' "$file")

fail() {
	cat >&2 <<TEXT
commit refused: $1

A commit here is one non-empty English subject line, then a blank line and the two
Co-Authored-By trailers, nothing else (AGENTS.md, "Commits"):

  Co-Authored-By: Codex (<model>) <noreply@openai.com>
  Co-Authored-By: Claude <model> <noreply@anthropic.com>

Fix the message and commit again. If the commit is already made, add the missing
part in a new commit — never amend or rewrite a commit.
TEXT
	exit 1
}

subject=$(printf '%s\n' "$message" | sed -n '1p')
[ -n "$subject" ] || fail "the first line, the subject, is empty"

rest=$(printf '%s\n' "$message" | sed '1d')
body=$(printf '%s\n' "$rest" | grep -v '^[[:space:]]*$' | grep -v '^[A-Za-z][A-Za-z-]*: ')
[ -z "$body" ] || fail "text after the subject that is not a trailer (no body): $(printf '%s' "$body" | sed -n '1p')"

trailer() {
	printf '%s\n' "$rest" | grep -qi "^Co-Authored-By: $1 .*<[^>]*@[^>]*>[[:space:]]*\$"
}
trailer Codex || fail "the Co-Authored-By trailer for Codex is missing"
trailer Claude || fail "the Co-Authored-By trailer for Claude is missing"
