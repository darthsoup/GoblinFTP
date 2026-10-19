import argparse
import re
import subprocess
import sys
from urllib.parse import quote


def release_exists(tag):
    result = subprocess.run(
        [
            "gh", "api", "--include", "--silent",
            f"repos/{{owner}}/{{repo}}/releases/tags/{quote(tag, safe='')}",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode == 0:
        return True
    if re.match(r"HTTP/\S+ 404(?:\s|$)", result.stdout):
        # Lookup by tag only guarantees published releases; also preserve drafts.
        result = subprocess.run(
            [
                "gh", "api", "--paginate", "--jq", ".[].tag_name",
                "repos/{owner}/{repo}/releases?per_page=100",
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode == 0:
            return tag in result.stdout.splitlines()
    raise RuntimeError(f"Could not check release {tag}: {result.stderr.strip()}")


def ensure_release(tag, notes_file):
    if release_exists(tag):
        print(f"Release {tag} already exists; keeping its title, notes, and status.")
        return

    command = [
        "gh", "release", "create", tag,
        "--verify-tag", "--title", tag,
        "--notes-file", notes_file, "--generate-notes",
    ]
    if "-" in tag.split("+", 1)[0]:
        command.extend(["--prerelease", "--latest=false"])
    result = subprocess.run(command, capture_output=True, text=True, check=False)
    if result.returncode == 0:
        print(result.stdout, end="")
        return

    # Another publisher can create the release after the initial lookup.
    if release_exists(tag):
        print(f"Release {tag} was created concurrently; keeping it unchanged.")
        return
    raise RuntimeError(f"Could not create release {tag}: {result.stderr.strip()}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("tag")
    parser.add_argument("notes_file")
    args = parser.parse_args()
    try:
        ensure_release(args.tag, args.notes_file)
    except RuntimeError as error:
        print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
