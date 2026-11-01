#!/usr/bin/env python3
"""Collect every selected run, including runs without a resolved task.

Run only on the trusted private collector. Observatory supplies all receipt,
schema, hash and platform validation; this adapter only sequences its commands.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def read_discovery(path):
    with Path(path).open("rb") as handle:
        raw = handle.read(16 * 1024 * 1024 + 1)
    if len(raw) > 16 * 1024 * 1024:
        raise ValueError("discovery document exceeds 16 MiB")

    def unique(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate discovery field")
            result[key] = value
        return result

    value = json.loads(raw, object_pairs_hook=unique)
    if not isinstance(value, dict) or value.get("schema") != "observatory.discovery/1":
        raise ValueError("unsupported discovery schema")
    if type(value.get("complete")) is not bool:
        raise ValueError("discovery completeness must be explicit")
    runs = value.get("selected_runs")
    if not isinstance(runs, list) or len(runs) > 20000:
        raise ValueError("invalid selected run list")
    selected = set()
    for run in runs:
        if not isinstance(run, dict):
            raise ValueError("invalid selected run")
        repository, run_id = run.get("repository"), run.get("run_id")
        if (not isinstance(repository, str) or len(repository) > 256
                or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}", repository)):
            raise ValueError("invalid selected repository")
        if type(run_id) is not int or not 0 < run_id <= 2**53 - 1:
            raise ValueError("invalid selected run ID")
        selected.add((repository, run_id))
    return value, sorted(selected), hashlib.sha256(raw).hexdigest()


def collect(discovery, state, observatory, outcome):
    value, runs, digest = read_discovery(discovery)
    state, output = Path(state).resolve(), Path(outcome).resolve()
    binary = str(Path(observatory).resolve())
    state.mkdir(parents=True, exist_ok=True, mode=0o700)
    output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if output.exists():
        raise ValueError("use a new outcome path; retained evidence is immutable")
    outcomes = []
    with (state / ".pilot.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        logs = Path(tempfile.mkdtemp(prefix="collection-", dir=output.parent))

        def invoke(arguments, name):
            with (logs / name).open("xb") as handle:
                try:
                    return subprocess.run([binary, "--state", str(state), *arguments],
                                          stdout=handle, stderr=subprocess.STDOUT,
                                          check=False, timeout=1800).returncode
                except subprocess.TimeoutExpired:
                    return 124

        for index, (repository, run_id) in enumerate(runs):
            code = invoke(["collect-github-executions", "--repository", repository,
                           "--run-id", str(run_id)], f"run-{index}.log")
            outcomes.append({"repository": repository, "run_id": run_id, "returncode": code})
        verified = invoke(["verify"], "verify.log") == 0
    complete = value.get("complete") is True and verified and all(item["returncode"] == 0 for item in outcomes)
    report = {"schema": "observatory.discovered-collection/1", "discovery_sha256": digest,
              "discovery_complete": value.get("complete") is True,
              "store_verified": verified, "complete": complete, "outcomes": outcomes}
    with output.open("x") as handle:
        json.dump(report, handle, indent=2)
        handle.write("\n")
    return complete


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--discovery", type=Path, required=True)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--observatory", type=Path, required=True)
    parser.add_argument("--outcome", type=Path, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    try:
        complete = collect(args.discovery, args.state, args.observatory, args.outcome)
    except (OSError, ValueError, TypeError, KeyError) as error:
        # The workflow redirects diagnostics to a private log.
        parser.exit(2, f"collection adapter: {error}\n")
    print("Collection complete." if complete else "Collection incomplete; inspect retained private coverage and logs.")
    return 0 if complete else 1


if __name__ == "__main__":
    raise SystemExit(main())
