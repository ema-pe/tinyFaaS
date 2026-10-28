#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = ["requests"]
# ///
"""Manage functions on a tinyFaaS instance through its management API."""

import argparse
import base64
import io
import os
import sys
import zipfile

import requests


def zip_folder(folder):
    """Returns the base64 encoded zip of the folder content."""
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        for root, _, files in os.walk(folder):
            for name in files:
                full = os.path.join(root, name)
                zf.write(full, os.path.relpath(full, folder))
    return base64.b64encode(buf.getvalue()).decode("ascii")


def upload(args):
    if not os.path.isdir(args.folder):
        sys.exit(f"error: {args.folder} is not a folder")

    return requests.post(
        f"{args.url}/upload",
        json={
            "name": args.name,
            "env": args.env,
            "threads": args.threads,
            "zip": zip_folder(args.folder),
            "envs": args.envs,
        },
    )


def upload_url(args):
    return requests.post(
        f"{args.url}/uploadURL",
        json={
            "name": args.name,
            "env": args.env,
            "threads": args.threads,
            "url": args.zip_url,
            "subfolder_path": args.subfolder,
            "envs": args.envs,
        },
    )


def delete(args):
    return requests.post(f"{args.url}/delete", json={"name": args.name})


def list_functions(args):
    return requests.get(f"{args.url}/list")


def logs(args):
    params = {"name": args.name} if args.name else None
    return requests.get(f"{args.url}/logs", params=params)


def wipe(args):
    return requests.post(f"{args.url}/wipe")


def add_function_args(parser):
    parser.add_argument("name", help="name of the function")
    parser.add_argument("env", help="runtime environment (python3, nodejs, binary)")
    parser.add_argument("threads", type=int, help="number of function handlers")
    parser.add_argument(
        "-e",
        "--envvar",
        dest="envs",
        action="append",
        default=[],
        metavar="KEY=VALUE",
        help="environment variable for the function (repeatable)",
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--url",
        default="http://localhost:8080",
        help="management service URL (default: %(default)s)",
    )
    sub = parser.add_subparsers(required=True, metavar="COMMAND")

    p = sub.add_parser("upload", help="upload a function from a local folder")
    p.add_argument("folder", help="folder with the function code")
    add_function_args(p)
    p.set_defaults(func=upload)

    p = sub.add_parser("upload-url", help="upload a function from a zip at a URL")
    p.add_argument("zip_url", metavar="url", help="URL of the zip file")
    p.add_argument("subfolder", help="folder with the code inside the zip")
    add_function_args(p)
    p.set_defaults(func=upload_url)

    p = sub.add_parser("delete", help="delete a function")
    p.add_argument("name", help="name of the function")
    p.set_defaults(func=delete)

    p = sub.add_parser("list", help="list functions")
    p.set_defaults(func=list_functions)

    p = sub.add_parser("logs", help="print function logs")
    p.add_argument("name", nargs="?", help="only logs of this function")
    p.set_defaults(func=logs)

    p = sub.add_parser("wipe", help="delete all functions")
    p.set_defaults(func=wipe)

    args = parser.parse_args()

    try:
        resp = args.func(args)
    except requests.RequestException as e:
        sys.exit(f"error: {e}")

    if resp.text:
        print(resp.text, end="" if resp.text.endswith("\n") else "\n")
    if not resp.ok:
        sys.exit(f"error: HTTP {resp.status_code}")


if __name__ == "__main__":
    main()
