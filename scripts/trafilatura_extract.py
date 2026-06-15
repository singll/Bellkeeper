#!/usr/bin/env python3
"""Trafilatura extraction wrapper for Bellkeeper.

Trafilatura 2.0 removed __main__.py, so `python3 -m trafilatura` no longer works.
This wrapper uses the Python API to achieve the same result.

Usage: python3 trafilatura_extract.py <url> [--timeout SECONDS] [--user-agent UA] [--headers JSON]
Output: JSON to stdout with {title, author, url, date, raw_text} or {error: ...}
"""
import sys
import json
import trafilatura


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "URL argument required"}))
        sys.exit(1)

    url = sys.argv[1]
    timeout = 30
    user_agent = None
    extra_headers = {}
    for i, arg in enumerate(sys.argv):
        if arg == "--timeout" and i + 1 < len(sys.argv):
            timeout = int(sys.argv[i + 1])
        elif arg == "--user-agent" and i + 1 < len(sys.argv):
            user_agent = sys.argv[i + 1]
        elif arg == "--headers" and i + 1 < len(sys.argv):
            try:
                extra_headers = json.loads(sys.argv[i + 1])
            except json.JSONDecodeError:
                pass

    try:
        headers = extra_headers.copy() if extra_headers else {}
        if user_agent:
            headers["User-Agent"] = user_agent
        downloaded = trafilatura.fetch_url(url, headers=headers if headers else None)
        if not downloaded:
            print(json.dumps({"error": "fetch failed", "url": url}))
            sys.exit(1)

        result = trafilatura.extract(
            downloaded,
            output_format="json",
            include_tables=True,
            include_links=True,
        )
        if not result:
            print(json.dumps({"error": "extraction failed", "url": url}))
            sys.exit(1)

        sys.stdout.write(result)
    except Exception as e:
        print(json.dumps({"error": str(e), "url": url}))
        sys.exit(1)


if __name__ == "__main__":
    main()
