#!/usr/bin/env python3
"""Render the root-only Nginx credential include on Raven."""

import base64
import os
from pathlib import Path
import shlex
import tempfile


def render(environment, destination):
    settings = {}
    for line in environment.read_text().splitlines():
        name, _, value = line.strip().partition("=")
        if name in {"ADMIN_USERNAME", "ADMIN_PASSWORD"}:
            words = shlex.split(value)
            if len(words) != 1 or not words[0] or name in settings:
                raise SystemExit(f"Invalid or duplicate {name}")
            settings[name] = words[0]

    try:
        username = settings["ADMIN_USERNAME"]
        password = settings["ADMIN_PASSWORD"]
    except KeyError as error:
        raise SystemExit(f"Missing {error.args[0]}") from None
    if ":" in username:
        raise SystemExit("ADMIN_USERNAME cannot contain a colon")

    credentials = base64.b64encode(f"{username}:{password}".encode()).decode("ascii")
    with tempfile.NamedTemporaryFile(mode="w", dir=destination.parent, delete=False) as output:
        temp_path = Path(output.name)
        try:
            output.write(f'proxy_set_header Authorization "Basic {credentials}";\n')
            output.flush()
            os.replace(temp_path, destination)
        finally:
            temp_path.unlink(missing_ok=True)


if __name__ == "__main__":
    directory = Path("/etc/nominatim-cache")
    render(directory / "backend.env", directory / "ops-upstream-auth.conf")
