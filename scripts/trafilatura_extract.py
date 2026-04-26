#!/usr/bin/env python3
"""Trafilatura extraction wrapper for Bellkeeper.

Trafilatura 2.0 removed __main__.py, so `python3 -m trafilatura` no longer works.
This wrapper uses the Python API to achieve the same result.

Usage: python3 trafilatura_extract.py <url> [--timeout SECONDS]
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
    for i, arg in enumerate(sys.argv):
        if arg == "--timeout" and i + 1 < len(sys.argv):
            timeout = int(sys.argv[i + 1])

    try:
        downloaded = trafilatura.fetch_url(url)
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

        # trafilatura.extract with output_format="json" returns a JSON string
        sys.stdout.write(result)
    except Exception as e:
        print(json.dumps({"error": str(e), "url": url}))
        sys.exit(1)


if __name__ == "__main__":
    main()
