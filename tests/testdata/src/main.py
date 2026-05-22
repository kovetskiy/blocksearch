"""Canonical 4-space Python. The baseline normal case."""


import os
import sys
from typing import List


def main(argv: List[str]) -> int:
    if not argv:
        print("no args")
        return 1

    for arg in argv:
        if arg.startswith("--"):
            handle_flag(arg)
        else:
            handle_positional(arg)

    return 0


def handle_flag(flag: str) -> None:
    name, _, value = flag.partition("=")
    if value:
        print(f"{name} = {value}")
    else:
        print(f"{name} (no value)")


def handle_positional(value: str) -> None:
    try:
        number = int(value)
    except ValueError:
        print(f"not a number: {value}")
    else:
        print(f"number: {number}")


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
