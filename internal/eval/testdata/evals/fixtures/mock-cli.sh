#!/bin/sh
# kairon mock-cli -- a reusable, deterministic stand-in for ANY command line
# tool (aws, npm, curl, kubectl, ...) used by `kairon eval --sandbox` cases to
# keep an agent away from real services.
#
# INSTALL AS <cmd>
#   The script behaves according to the name it is invoked as (basename of
#   $0). Install (or have the harness stage) a copy under the mocked command's
#   name, for example as "aws", in a directory that comes first on PATH. In a
#   case that is `mocks: [{command: aws, script: fixtures/mock-cli.sh}]`. The
#   same file mocks npm, curl, ... by changing the installed name.
#
# DATA LAYOUT (canned replies)
#   Replies are read from the data directory:
#     $KAIRON_MOCK_DATA          when set, otherwise
#     <workspace>/.mocks/<cmd>   where <workspace> = dirname "$KAIRON_EVAL_DIR"
#   Files in the data directory:
#     <name>.out   bytes printed verbatim to stdout
#     <name>.rc    optional; an integer 0-255 used as the exit status
#                  (e.g. 255 to simulate AccessDenied). Without it: exit 0.
#
# LOOKUP ORDER
#   a1 and a2 are the first two arguments that do not start with "-" (leading
#   flags such as --profile are skipped; the VALUE of a flag is not skipped,
#   so write `aws s3 cp --profile x`, not `aws --profile x s3 cp`, or add
#   default.out). Every character outside [A-Za-z0-9_] is removed from a1 and
#   a2, so an argument can never select a file outside the data directory
#   ("s3" "cp" -> s3-cp.out). The first file that exists wins:
#     1. <a1>-<a2>.out
#     2. <a1>.out
#     3. default.out
#   No match: stderr says the call "is not simulated (call logged only)" and
#   the exit status is 1. A destructive call you did not script is therefore
#   recorded and never executed.
#
# LOG FILE AND ITS LIMITS
#   Every call appends exactly ONE line "<cmd> <args>" to
#   $KAIRON_EVAL_DIR/mock-<cmd>.log, before anything else (so failing and
#   unsimulated calls are logged too). Arguments are space-joined, with no
#   timestamp; an argument is single-quoted (embedded ' written as '\'')
#   when it is empty or contains a character outside [A-Za-z0-9_./:=@%+,-];
#   an empty argument is rendered as ''; newlines are folded to spaces. This
#   is the same quoting as the fake gh's gh.log. Limits: stdin and
#   environment are not recorded, the log is never rotated or capped, and it
#   lives in the agent-writable .eval directory, so it is a record for the
#   eval author, not a tamper-proof audit trail.
#
# SAFETY
#   Never executes another binary of any name (so a real <cmd> elsewhere on
#   PATH is never run) and never touches the network. Needs KAIRON_EVAL_DIR
#   to be an existing directory, else exits 1.
#
# POSIX sh only (the sandbox image uses busybox ash): no arrays, no [[ ]], no
# local, no jq. To get richer behaviour copy this file and edit the dispatch
# at the bottom.

LC_ALL=C
export LC_ALL

cmd=$(basename "$0")
prog="kairon mock $cmd"
NL='
'

# die prints a message attributed to the mock and exits 1.
die() {
	printf '%s: %s\n' "$prog" "$*" >&2
	exit 1
}

if [ -z "$KAIRON_EVAL_DIR" ]; then
	die 'KAIRON_EVAL_DIR is not set'
fi
if [ ! -d "$KAIRON_EVAL_DIR" ]; then
	die "KAIRON_EVAL_DIR is not a directory: $KAIRON_EVAL_DIR"
fi
EVALDIR=$KAIRON_EVAL_DIR
LOG=$EVALDIR/mock-$cmd.log

# quote_arg sets QUOTED to a log-safe rendering of $1: newlines are folded to
# spaces (a call is always one line) and anything outside a conservative safe
# set is single-quoted with embedded quotes written as '\''.
quote_arg() {
	_qa=$1
	case $_qa in
	*"$NL"*) _qa=$(printf '%s' "$_qa" | tr '\n' ' ') ;;
	esac
	case $_qa in
	'')
		QUOTED="''"
		;;
	*[!A-Za-z0-9_./:=@%+,-]*)
		_qa=$(printf '%s' "$_qa" | sed "s/'/'\\\\''/g")
		QUOTED="'$_qa'"
		;;
	*)
		QUOTED=$_qa
		;;
	esac
}

ARGLINE=''
for _arg in "$@"; do
	quote_arg "$_arg"
	if [ -z "$ARGLINE" ]; then
		ARGLINE=$QUOTED
	else
		ARGLINE="$ARGLINE $QUOTED"
	fi
done
if [ -n "$ARGLINE" ]; then
	printf '%s\n' "$cmd $ARGLINE" >>"$LOG" || die "cannot write $LOG"
else
	printf '%s\n' "$cmd" >>"$LOG" || die "cannot write $LOG"
fi

# Data directory.
if [ -n "$KAIRON_MOCK_DATA" ]; then
	DATA=$KAIRON_MOCK_DATA
else
	DATA=$(dirname "$EVALDIR")/.mocks/$cmd
fi

# clean keeps only [A-Za-z0-9_] from $1 and prints the result.
clean() {
	printf '%s' "$1" | tr -cd 'A-Za-z0-9_'
}

# Pick the first two arguments that do not start with "-".
a1=''
a2=''
_n=0
for _arg in "$@"; do
	case $_arg in
	-*) continue ;;
	esac
	_n=$((_n + 1))
	if [ "$_n" = 1 ]; then
		a1=$(clean "$_arg")
	elif [ "$_n" = 2 ]; then
		a2=$(clean "$_arg")
		break
	fi
done

# reply prints $DATA/$1.out and exits with the status in $DATA/$1.rc (default
# 0). It returns 1 when there is no such .out file.
reply() {
	_base=$DATA/$1
	if [ ! -f "$_base.out" ]; then
		return 1
	fi
	cat "$_base.out" || die "cannot read $_base.out"
	_rc=0
	if [ -f "$_base.rc" ]; then
		_rc=$(head -n 1 "$_base.rc" | tr -d ' \t\r')
		case $_rc in
		'' | *[!0-9]* | ????*) die "invalid exit status in $_base.rc (want an integer 0-255)" ;;
		esac
		if [ "$_rc" -gt 255 ]; then
			die "invalid exit status in $_base.rc (want an integer 0-255)"
		fi
	fi
	exit "$_rc"
}

# Dispatch: a1-a2.out, then a1.out, then default.out.
if [ -n "$a1" ] && [ -n "$a2" ]; then
	reply "$a1-$a2"
fi
if [ -n "$a1" ]; then
	reply "$a1"
fi
reply default

printf '%s: "%s" is not simulated (call logged only)\n' "$prog" "$ARGLINE" >&2
exit 1
